package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"spotter/internal/app"
	"spotter/internal/planner"
	"spotter/internal/sse"
	"spotter/internal/storage"
	"spotter/internal/workspace"
)

func TestWorkspaceHTTP(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	broker := sse.NewBroker(logger)
	a := app.New(logger, nil, nil, nil, nil, broker)
	work, e := workspace.New(filepath.Join(t.TempDir(), "work.json"))
	if e != nil {
		t.Fatal(e)
	}
	handler := New(logger, a, broker).WithWorkspace(work, nil).Handler()
	for _, tc := range []struct {
		header, origin string
		status         int
	}{{"", "", 403}, {"workspace", "https://foreign.test", 403}, {"workspace", "http://example.com", 200}} {
		body := `{"version":0,"action":"capture","task":{"title":"Check current","minutes":90}}`
		r := httptest.NewRequest(http.MethodPost, "http://example.com/api/workspace", strings.NewReader(body))
		r.Header.Set("X-Spotter-Request", tc.header)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("GET", "http://example.com/api/workspace", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("Check current")) {
		t.Fatal(w.Body.String())
	}
	var out map[string]any
	if e = json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatal(e)
	}
	r = httptest.NewRequest("POST", "http://example.com/api/workspace", strings.NewReader(`{"version":0,"action":"capture","task":{"title":"Duplicate","minutes":90}}`))
	r.Header.Set("X-Spotter-Request", "workspace")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
}
func TestICSEscapingAndFolding(t *testing.T) {
	v := foldICS("SUMMARY:" + escapeICS(strings.Repeat("Привет", 30)+"\nBEGIN:VEVENT;test,\\"))
	for _, line := range strings.Split(v, "\r\n") {
		if len(line) > 75 {
			t.Fatal("long ICS line")
		}
	}
	if strings.Contains(v, "\nBEGIN:VEVENT") {
		t.Fatal("injection")
	}
}

func TestLegacyRoutesAndErrors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	broker := sse.NewBroker(logger)
	store := storage.JSONStore{Path: filepath.Join(t.TempDir(), "state.json")}
	a := app.New(logger, nil, planner.RuleBased{}, store, nil, broker)
	s := New(logger, a, broker)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/workspace", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	s.handleCalendarExport(w, httptest.NewRequest("GET", "/api/focus.ics", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	work, err := workspace.New(filepath.Join(t.TempDir(), "workspace.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.WithWorkspace(work, &workspace.Bridge{})
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{{"POST", "/api/state", "", 405}, {"GET", "/api/state", "", 200}, {"GET", "/api/refresh", "", 405}, {"POST", "/api/refresh", "", 200}, {"PUT", "/api/workspace", "", 405}, {"POST", "/api/workspace", "invalid", 400}, {"POST", "/api/workspace", "{} {}", 400}, {"POST", "/api/workspace", `{"action":"bad"}`, 400}, {"POST", "/api/focus.ics", "", 405}, {"GET", "/api/focus.ics", "", 400}, {"GET", "/api/workspace?date=2026-10-05", "", 200}} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Header.Set("X-Spotter-Request", "workspace")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
	for _, r := range []*http.Request{httptest.NewRequest("GET", "/api/state", nil), httptest.NewRequest("GET", "/api/state", nil)} {
		r.RemoteAddr = ""
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
		r.RemoteAddr = "127.0.0.1:42"
		r.Header.Set("X-Forwarded-For", "foreign")
		s.Handler().ServeHTTP(w, r)
	}
}
