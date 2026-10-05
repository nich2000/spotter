package platform

import (
	"context"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"spotter/internal/core"
)

func (a *API) authorized(r *http.Request) bool {
	cookie, err := r.Cookie("spotter_session")
	if err != nil {
		return false
	}
	var yes bool
	err = a.Store.DB.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM sessions WHERE token_hash=$1 AND created_at>now()-interval '12 hours' AND touched_at>now()-interval '30 minutes')", hash([]byte(cookie.Value))).Scan(&yes)
	return err == nil && yes
}
func (a *API) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.authorized(r) {
			problem(w, core.Fail("UNAUTHORIZED", 401, "Требуется вход"))
			return
		}
		cookie, _ := r.Cookie("spotter_session")
		_, err := a.Store.DB.Exec(r.Context(), "UPDATE sessions SET touched_at=now() WHERE token_hash=$1", hash([]byte(cookie.Value)))
		if err != nil {
			problem(w, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *API) session(w http.ResponseWriter, r *http.Request) {
	var exists bool
	if err := a.Store.DB.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM owner)").Scan(&exists); err != nil {
		problem(w, err)
		return
	}
	send(w, 200, core.Object{"authenticated": a.authorized(r), "setupRequired": !exists})
}
func (a *API) setup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Password string `json:"password"`
	}
	if err := body(w, r, &input); err != nil {
		problem(w, err)
		return
	}
	if len(input.Password) < 12 || len(input.Password) > 72 {
		problem(w, core.Fail("VALIDATION_FAILED", 422, "Пароль: от 12 до 72 байт"))
		return
	}
	h, err := bcrypt.GenerateFromPassword([]byte(input.Password), 12)
	if err != nil {
		problem(w, err)
		return
	}
	tag, err := a.Store.DB.Exec(r.Context(), "INSERT INTO owner VALUES(1,$1) ON CONFLICT DO NOTHING", string(h))
	if err != nil {
		problem(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		problem(w, core.Fail("OWNER_EXISTS", 409, "Владелец уже настроен"))
		return
	}
	a.issueSession(w, r)
}
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Password string `json:"password"`
	}
	if err := body(w, r, &input); err != nil {
		problem(w, err)
		return
	}
	var stored string
	err := a.Store.DB.QueryRow(r.Context(), "SELECT password_hash FROM owner WHERE id=1").Scan(&stored)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(stored), []byte(input.Password)) != nil {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
		problem(w, core.Fail("INVALID_CREDENTIALS", 401, "Неверный пароль"))
		return
	}
	a.issueSession(w, r)
}
func (a *API) issueSession(w http.ResponseWriter, r *http.Request) {
	token := core.ID() + core.ID()
	_, err := a.Store.DB.Exec(r.Context(), "INSERT INTO sessions(token_hash) VALUES($1)", hash([]byte(token)))
	if err != nil {
		problem(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "spotter_session", Value: token, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(a.Origin, "https:"), SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	send(w, 200, core.Object{"authenticated": true})
}
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("spotter_session"); err == nil {
		if _, err = a.Store.DB.Exec(r.Context(), "DELETE FROM sessions WHERE token_hash=$1", hash([]byte(cookie.Value))); err != nil {
			problem(w, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "spotter_session", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	send(w, 200, core.Object{"authenticated": false})
}
func (a *API) createEnrollment(w http.ResponseWriter, r *http.Request) {
	code := core.ID()
	_, err := a.Store.DB.Exec(r.Context(), "INSERT INTO enrollments VALUES($1,now()+interval '5 minutes')", hash([]byte(code)))
	if err != nil {
		problem(w, err)
		return
	}
	send(w, 200, core.Object{"code": code, "expiresInSeconds": 300})
}
func (a *API) enroll(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := body(w, r, &input); err != nil {
		problem(w, err)
		return
	}
	if len(input.Name) == 0 || len(input.Name) > 200 {
		problem(w, core.Fail("VALIDATION_FAILED", 422, "Укажите имя устройства"))
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		problem(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), "DELETE FROM enrollments WHERE code_hash=$1 AND expires_at>now()", hash([]byte(input.Code)))
	if err != nil {
		problem(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		problem(w, core.Fail("ENROLLMENT_EXPIRED", 403, "Код истёк или использован"))
		return
	}
	id, token := core.ID(), core.ID()+core.ID()
	_, err = tx.Exec(r.Context(), "INSERT INTO devices(id,name,token_hash) VALUES($1,$2,$3)", id, input.Name, hash([]byte(token)))
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		problem(w, err)
		return
	}
	send(w, 200, core.Object{"deviceId": id, "token": token})
}

type deviceKey struct{}

func (a *API) deviceAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		var id string
		err := a.Store.DB.QueryRow(r.Context(), "SELECT id FROM devices WHERE token_hash=$1 AND active", hash([]byte(token))).Scan(&id)
		if err != nil {
			problem(w, core.Fail("UNAUTHORIZED", 401, "Устройство не авторизовано"))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), deviceKey{}, id)))
	})
}
func (a *API) devices(w http.ResponseWriter, r *http.Request) {
	rows, err := a.Store.DB.Query(r.Context(), "SELECT id,name,active,created_at,last_seen FROM devices ORDER BY created_at")
	if err != nil {
		problem(w, err)
		return
	}
	defer rows.Close()
	out := []core.Object{}
	for rows.Next() {
		var id, name string
		var active bool
		var created time.Time
		var seen *time.Time
		if err = rows.Scan(&id, &name, &active, &created, &seen); err != nil {
			problem(w, err)
			return
		}
		out = append(out, core.Object{"id": id, "name": name, "active": active, "createdAt": created, "lastSeen": seen})
	}
	send(w, 200, core.Object{"items": out})
}
func (a *API) revokeDevice(w http.ResponseWriter, r *http.Request) {
	_, err := a.Store.DB.Exec(r.Context(), "UPDATE devices SET active=false WHERE id=$1", r.PathValue("id"))
	if err != nil {
		problem(w, err)
		return
	}
	send(w, 200, core.Object{"revoked": true})
}
func (a *API) consents(w http.ResponseWriter, r *http.Request) {
	rows, err := a.Store.DB.Query(r.Context(), "SELECT category,enabled,generation FROM consents")
	if err != nil {
		problem(w, err)
		return
	}
	defer rows.Close()
	out := []core.Object{}
	for rows.Next() {
		var category string
		var enabled bool
		var generation int64
		if err = rows.Scan(&category, &enabled, &generation); err != nil {
			problem(w, err)
			return
		}
		out = append(out, core.Object{"category": category, "enabled": enabled, "generation": generation})
	}
	send(w, 200, core.Object{"items": out})
}
func (a *API) saveConsent(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Category string `json:"category"`
		Enabled  bool   `json:"enabled"`
	}
	if err := body(w, r, &input); err != nil {
		problem(w, err)
		return
	}
	if input.Category != "health" && input.Category != "diary" {
		problem(w, core.Fail("VALIDATION_FAILED", 422, "Неизвестная категория"))
		return
	}
	_, err := a.Store.DB.Exec(r.Context(), "UPDATE consents SET enabled=$2,generation=generation+1 WHERE category=$1 AND enabled<>$2", input.Category, input.Enabled)
	if err != nil {
		problem(w, err)
		return
	}
	send(w, 200, core.Object{"saved": true})
}
