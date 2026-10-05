package platform

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"spotter/internal/core"
)

type API struct {
	Store  *Store
	Origin string
	Assets string
	Broker *Broker
	Files  *Files
}

func send(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, err error) {
	var e *core.Error
	if !errors.As(err, &e) {
		e = core.Fail("SERVICE_UNAVAILABLE", 503, "Сервис временно недоступен")
	}
	send(w, e.Status, core.Object{"apiVersion": "2", "error": e})
}
func body(w http.ResponseWriter, r *http.Request, dest any) error {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "application/json" {
		return core.Fail("UNSUPPORTED_MEDIA_TYPE", 415, "Нужен application/json")
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return core.Fail("REQUEST_TOO_LARGE", 413, "Превышен размер запроса")
	}
	return core.Decode(b, dest)
}
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, r *http.Request) { send(w, 200, core.Object{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Store.DB.Ping(r.Context()); err != nil {
			problem(w, err)
			return
		}
		send(w, 200, core.Object{"status": "ready"})
	})
	mux.HandleFunc("GET /api/v2/auth/session", a.session)
	mux.HandleFunc("POST /api/v2/auth/setup", a.setup)
	mux.HandleFunc("POST /api/v2/auth/login", a.login)
	mux.HandleFunc("POST /api/v2/auth/logout", a.logout)
	mux.HandleFunc("POST /api/v2/devices/enroll", a.enroll)
	mux.Handle("/api/v2/integrations/", a.deviceAuth(http.HandlerFunc(a.ingestion)))
	private := http.NewServeMux()
	private.HandleFunc("POST /api/v2/workspace/commands", a.command)
	private.HandleFunc("POST /api/v2/calendar/proposal", a.calendarProposal)
	private.HandleFunc("POST /api/v2/device-enrollments", a.createEnrollment)
	private.HandleFunc("GET /api/v2/devices", a.devices)
	private.HandleFunc("POST /api/v2/devices/{id}/revoke", a.revokeDevice)
	private.HandleFunc("GET /api/v2/consents", a.consents)
	private.HandleFunc("POST /api/v2/consents", a.saveConsent)
	private.HandleFunc("GET /api/v2/events", a.events)
	private.HandleFunc("POST /api/v2/files", a.upload)
	private.HandleFunc("GET /api/v2/files/{id}", a.download)
	private.HandleFunc("GET /api/v2/sources", a.sources)
	private.HandleFunc("GET /api/v2/", a.read)
	mux.Handle("/api/v2/", a.auth(private))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		problem(w, core.Fail("CLIENT_UPGRADE_REQUIRED", 409, "Используйте API v2"))
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		problem(w, core.Fail("CLIENT_UPGRADE_REQUIRED", 409, "Используйте API v2"))
	})
	mux.HandleFunc("/", a.static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'")
		origin, _ := url.Parse(a.Origin)
		if origin == nil || r.Host != origin.Host {
			problem(w, core.Fail("HOST_REJECTED", 403, "Недопустимый Host"))
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if v := r.Header.Get("Origin"); v != "" && v != a.Origin {
				problem(w, core.Fail("ORIGIN_REJECTED", 403, "Недопустимый Origin"))
				return
			}
			if !strings.HasPrefix(r.URL.Path, "/api/v2/integrations/") && r.URL.Path != "/api/v2/devices/enroll" && r.Header.Get("X-Spotter-Request") != "workspace" {
				problem(w, core.Fail("REQUEST_HEADER_REQUIRED", 403, "Отсутствует защитный заголовок"))
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func (a *API) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	if strings.Contains(filepath.Base(r.URL.Path), ".") {
		http.FileServer(http.Dir(a.Assets)).ServeHTTP(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(a.Assets, "index.html"))
}
func (a *API) command(w http.ResponseWriter, r *http.Request) {
	var c core.Command
	if err := body(w, r, &c); err != nil {
		problem(w, err)
		return
	}
	if c.Command == "wellbeing.save" {
		var enabled bool
		if err := a.Store.DB.QueryRow(r.Context(), "SELECT enabled FROM consents WHERE category='diary'").Scan(&enabled); err != nil || !enabled {
			problem(w, core.Fail("CONSENT_REQUIRED", 403, "Включите хранение дневника в настройках"))
			return
		}
	}
	receipt, err := a.Store.Execute(r.Context(), c)
	if err != nil {
		problem(w, err)
		return
	}
	send(w, 200, receipt)
}
func values(m map[string]core.Object) []core.Object {
	out := make([]core.Object, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return stringValue(out[i], "id") < stringValue(out[j], "id") })
	return out
}
func stringValue(m core.Object, k string) string { v, _ := m[k].(string); return v }
func (a *API) read(w http.ResponseWriter, r *http.Request) {
	state, token, err := a.Store.Snapshot(r.Context(), r.URL.Query().Get("readToken"))
	if err != nil {
		problem(w, err)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/")
	var data any
	date := r.URL.Query().Get("date")
	if date == "" {
		loc, _ := time.LoadLocation(stringValue(state.Settings, "zone"))
		date = time.Now().In(loc).Format("2006-01-02")
	}
	switch {
	case path == "capabilities":
		cmds := []core.Object{}
		for _, name := range core.Commands {
			cmds = append(cmds, core.Object{"name": name, "available": name != "plan.close", "blockedBy": []string{}})
		}
		data = core.Object{"apiVersion": "2", "schemaVersion": 2, "validationProfile": "local-1", "commands": cmds, "policyVersions": policyVersions(), "legacyPolicy": core.Object{"ready": 15, "wip": 3, "mit": 3, "weekly": 5}, "features": core.Object{"timer": false, "recurrences": false, "historicalReports": false, "files": true, "planning": true, "planningPolicy": core.Object{"scheduledKinds": []string{"date_range", "timed"}, "deadline": "warn", "completed": "linked_only", "dateRangeEnd": "inclusive", "startOnly": "marker"}}, "migrationStatus": state.Migration}
	case path == "workspace":
		data = core.Object{"workBlocks": values(state.WorkBlocks), "calendarSources": values(state.CalendarSources), "tasks": values(state.Tasks), "candidates": values(state.Candidates), "projects": values(state.Projects), "plans": values(state.Plans), "settings": state.Settings, "days": state.Days, "migration": state.Migration, "readToken": token}
	case path == "tasks" || path == "inbox":
		items := []core.Object{}
		for _, t := range values(state.Tasks) {
			if path == "inbox" && t["lifecycleState"] == "done" {
				continue
			}
			if q := r.URL.Query().Get("q"); q != "" && !strings.Contains(strings.ToLower(stringValue(t, "title")), strings.ToLower(q)) {
				continue
			}
			items = append(items, t)
		}
		data = core.Object{"items": items, "readToken": token, "page": core.Object{"total": len(items), "nextCursor": nil}}
	case strings.HasPrefix(path, "tasks/"):
		id := strings.TrimPrefix(path, "tasks/")
		t, ok := state.Tasks[id]
		if !ok {
			problem(w, core.Fail("ENTITY_NOT_FOUND", 404, "Задача не найдена"))
			return
		}
		events := []core.Object{}
		for _, e := range state.Events {
			if e["entityId"] == id {
				events = append(events, e)
			}
		}
		data = core.Object{"task": t, "subtasks": t["subtasks"], "comments": t["comments"], "history": events}
	case path == "projects":
		data = core.Object{"items": values(state.Projects), "readToken": token}
	case path == "plans":
		data = core.Object{"items": values(state.Plans), "readToken": token}
	case path == "settings":
		data = state.Settings
	case path == "migration":
		data = state.Migration
	case path == "workday":
		plans := []core.Object{}
		var active any
		for _, p := range state.Plans {
			if p["date"] == date {
				plans = append(plans, p)
				if p["status"] == "active" {
					active = p
				}
			}
		}
		data = core.Object{"date": date, "plans": plans, "activePlan": active, "focus": nil, "timer": nil, "schedule": core.Object{"status": "unavailable", "reason": "CALENDAR_COVERAGE_UNKNOWN", "capacityMinutes": nil}}
	case path == "wellbeing":
		data = core.Object{"items": values(state.Days)}
	case strings.HasPrefix(path, "wellbeing/"):
		day := state.Days[strings.TrimPrefix(path, "wellbeing/")]
		data = core.Object{"record": day, "rings": unavailable(), "measurements": []any{}}
	case path == "reports":
		data = core.Object{"summary": unavailable(), "coverage": state.Migration, "events": state.Events, "policyVersions": policyVersions(), "reportToken": token}
	case strings.HasPrefix(path, "operations/"):
		var b []byte
		err = a.Store.DB.QueryRow(r.Context(), "SELECT receipt FROM operations WHERE id=$1", strings.TrimPrefix(path, "operations/")).Scan(&b)
		if err != nil {
			problem(w, core.Fail("OPERATION_NOT_FOUND", 404, "Квитанция не найдена"))
			return
		}
		send(w, 200, json.RawMessage(b))
		return
	default:
		problem(w, core.Fail("ENTITY_NOT_FOUND", 404, "Маршрут не найден"))
		return
	}
	send(w, 200, core.Object{"apiVersion": "2", "workspaceRevision": state.Revision, "serverNow": time.Now().UTC(), "sourceSnapshotId": state.SourceSnapshotID, "data": data, "warnings": []any{}})
}
func unavailable() core.Object {
	return core.Object{"value": nil, "status": "unavailable", "reasons": []string{"POLICY_UNRESOLVED"}, "decisionIds": []string{"R1", "R2", "R3", "R6", "R7", "R8", "R12"}}
}
func policyVersions() []core.Object {
	out := []core.Object{}
	for _, id := range []string{"R1", "R2", "R3", "R4", "R5", "R6", "R7", "R8", "R9", "R10", "R11", "R12", "R13"} {
		out = append(out, core.Object{"id": id, "status": "proposal", "version": nil, "value": nil})
	}
	return out
}
func (a *API) events(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	var revision int64 = -1
	var sourceSnapshot string
	for {
		if !a.authorized(r) {
			return
		}
		s, err := a.Store.Read(r.Context())
		if err != nil {
			return
		}
		if revision != s.Revision || sourceSnapshot != s.SourceSnapshotID {
			b, _ := json.Marshal(core.Object{"revision": s.Revision, "sourceSnapshotId": s.SourceSnapshotID})
			_, _ = w.Write([]byte("event: reset\ndata: " + string(b) + "\n\n"))
			revision = s.Revision
			sourceSnapshot = s.SourceSnapshotID
		} else {
			_, _ = w.Write([]byte(": keepalive\n\n"))
		}
		f.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
