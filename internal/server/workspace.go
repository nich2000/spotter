package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"spotter/internal/workspace"
)

func (s *Server) WithWorkspace(w *workspace.Service, b *workspace.Bridge) *Server {
	s.workspace = w
	s.healthBridge = b
	return s
}
func (s *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.workspace == nil {
		http.Error(w, "Рабочая панель не настроена", 503)
		return
	}
	if r.Method == http.MethodPost {
		// A custom header prevents cross-site form writes; never enable CORS here.
		if r.Header.Get("X-Spotter-Request") != "workspace" {
			http.Error(w, "forbidden", 403)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host {
				http.Error(w, "forbidden", 403)
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		var c workspace.Command
		if err := dec.Decode(&c); err != nil {
			http.Error(w, "Некорректный запрос", 400)
			return
		}
		if dec.Decode(&struct{}{}) != io.EOF {
			http.Error(w, "Некорректный запрос", 400)
			return
		}
		if err := s.workspace.Apply(c, s.app.State(), time.Now()); err != nil {
			code := 400
			if errors.Is(err, workspace.ErrConflict) {
				code = 409
			}
			http.Error(w, err.Error(), code)
			return
		}
	} else if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	d := s.workspace.Snapshot()
	date := r.URL.Query().Get("date")
	if date == "" {
		zone, _ := time.LoadLocation(d.Settings.Zone)
		date = time.Now().In(zone).Format("2006-01-02")
	}
	state := s.app.State()
	bridge := "Источник iPhone не настроен"
	if s.healthBridge != nil {
		bridge = s.healthBridge.Message()
	}
	writeJSON(w, struct {
		Data         workspace.Data        `json:"data"`
		Candidates   []workspace.Candidate `json:"candidates"`
		Schedule     workspace.Schedule    `json:"schedule"`
		HealthBridge string                `json:"healthBridge"`
	}{d, workspace.Candidates(state, d), workspace.Build(d, state, date, time.Now()), bridge})
}
func (s *Server) handleCalendarExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	if s.workspace == nil {
		http.Error(w, "not configured", 503)
		return
	}
	now := time.Now()
	plan := workspace.Build(s.workspace.Snapshot(), s.app.State(), r.URL.Query().Get("date"), now)
	count := 0
	for _, b := range plan.Blocks {
		if b.Kind == "focus" {
			count++
		}
	}
	if count == 0 {
		http.Error(w, "Нет временных блоков для экспорта. Проверьте MIT и календарь.", 400)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="spotter-focus.ics"`)
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Personal Spotter//Focus//RU\r\n")
	stamp := func(t time.Time) string { return t.UTC().Format("20060102T150405Z") }
	for _, b := range plan.Blocks {
		if b.Kind != "focus" {
			continue
		}
		fmt.Fprintf(w, "BEGIN:VEVENT\r\nUID:%s-%s@spotter\r\nDTSTAMP:%s\r\nDTSTART:%s\r\nDTEND:%s\r\n", b.TaskID, stamp(b.Start), stamp(now), stamp(b.Start), stamp(b.End))
		fmt.Fprint(w, foldICS("SUMMARY:"+escapeICS(b.Title))+"\r\nEND:VEVENT\r\n")
	}
	fmt.Fprint(w, "END:VCALENDAR\r\n")
}
func escapeICS(v string) string {
	return strings.NewReplacer("\\", "\\\\", "\r", "", "\n", "\\n", ";", "\\;", ",", "\\,").Replace(v)
}
func foldICS(v string) string {
	var b strings.Builder
	n := 0
	for _, r := range v {
		size := len(string(r))
		if n+size > 73 {
			b.WriteString("\r\n ")
			n = 1
		}
		b.WriteRune(r)
		n += size
	}
	return b.String()
}
