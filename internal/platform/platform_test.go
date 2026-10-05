package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"golang.org/x/crypto/bcrypt"
	"spotter/internal/core"
	"spotter/internal/model"
)

func mockStore(t *testing.T) (*Store, pgxmock.PgxPoolIface) {
	t.Helper()
	m, e := pgxmock.NewPool()
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e = m.ExpectationsWereMet(); e != nil {
			t.Error(e)
		}
		m.Close()
	})
	return &Store{DB: m}, m
}
func raw(v any) []byte { b, _ := json.Marshal(v); return b }
func TestStoreAtomicReceipt(t *testing.T) {
	s, m := mockStore(t)
	state := core.NewState()
	c := core.Command{OperationID: "op", Command: "task.create", Payload: core.Object{"title": "New"}}
	m.ExpectBegin()
	m.ExpectQuery("SELECT state").WillReturnRows(pgxmock.NewRows([]string{"state"}).AddRow(raw(state)))
	m.ExpectQuery("SELECT hash,receipt").WithArgs("op").WillReturnError(pgx.ErrNoRows)
	m.ExpectExec("UPDATE workspace").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	m.ExpectExec("INSERT INTO operations").WithArgs("op", hash(raw(c)), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	m.ExpectExec("INSERT INTO outbox").WithArgs(int64(1)).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	m.ExpectCommit()
	m.ExpectRollback()
	r, err := s.Execute(context.Background(), c)
	if err != nil || r.CommittedRevision != 1 {
		t.Fatal(r, err)
	}
	m.ExpectBegin()
	m.ExpectQuery("SELECT state").WillReturnRows(pgxmock.NewRows([]string{"state"}).AddRow(raw(state)))
	m.ExpectQuery("SELECT hash,receipt").WithArgs("op").WillReturnRows(pgxmock.NewRows([]string{"hash", "receipt"}).AddRow(hash(raw(c)), raw(r)))
	m.ExpectRollback()
	again, err := s.Execute(context.Background(), c)
	if err != nil || again.Result["taskId"] != r.Result["taskId"] {
		t.Fatal(again, err)
	}
	c.Payload["title"] = "changed"
	m.ExpectBegin()
	m.ExpectQuery("SELECT state").WillReturnRows(pgxmock.NewRows([]string{"state"}).AddRow(raw(state)))
	m.ExpectQuery("SELECT hash,receipt").WithArgs(pgxmock.AnyArg()).WillReturnRows(pgxmock.NewRows([]string{"hash", "receipt"}).AddRow("different", raw(r)))
	m.ExpectRollback()
	if _, err = s.Execute(context.Background(), c); err == nil {
		t.Fatal("id reuse")
	}
}
func TestStoreRollbackAndSnapshots(t *testing.T) {
	s, m := mockStore(t)
	state := core.NewState()
	c := core.Command{OperationID: "op", Command: "task.create", Payload: core.Object{"title": "New"}}
	m.ExpectBegin()
	m.ExpectQuery("SELECT state").WillReturnRows(pgxmock.NewRows([]string{"state"}).AddRow(raw(state)))
	m.ExpectQuery("SELECT hash,receipt").WithArgs(pgxmock.AnyArg()).WillReturnError(pgx.ErrNoRows)
	m.ExpectExec("UPDATE workspace").WithArgs(pgxmock.AnyArg()).WillReturnError(errors.New("disk"))
	m.ExpectRollback()
	if _, err := s.Execute(context.Background(), c); err == nil {
		t.Fatal("storage failure hidden")
	}
	m.ExpectQuery("SELECT state").WillReturnRows(pgxmock.NewRows([]string{"state"}).AddRow(raw(state)))
	m.ExpectExec("INSERT INTO read_snapshots").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	_, token, err := s.Snapshot(context.Background(), "")
	if err != nil || token == "" {
		t.Fatal(err)
	}
	m.ExpectQuery("SELECT state").WithArgs(token).WillReturnRows(pgxmock.NewRows([]string{"state"}).AddRow(raw(state)))
	if _, _, err = s.Snapshot(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	m.ExpectQuery("SELECT state").WithArgs("expired").WillReturnError(pgx.ErrNoRows)
	if _, _, err = s.Snapshot(context.Background(), "expired"); err == nil {
		t.Fatal("expired")
	}
	m.ExpectBegin()
	m.ExpectExec("SELECT pg_advisory").WillReturnResult(pgxmock.NewResult("SELECT", 1))
	m.ExpectExec("CREATE TABLE").WillReturnResult(pgxmock.NewResult("CREATE", 1))
	m.ExpectExec("INSERT INTO workspace").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	m.ExpectCommit()
	m.ExpectRollback()
	if err = s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func call(handler http.Handler, method, path string, b []byte, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost:18080"+path, bytes.NewReader(b))
	r.Host = "localhost:18080"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Spotter-Request", "workspace")
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}
func TestHTTPGuards(t *testing.T) {
	a := API{Origin: "http://localhost:18080", Assets: t.TempDir()}
	_ = os.WriteFile(filepath.Join(a.Assets, "index.html"), []byte("APP"), 0600)
	h := a.Handler()
	for _, tc := range []struct {
		method, path string
		headers      map[string]string
		status       int
	}{{"GET", "/livez", nil, 200}, {"POST", "/api/v2/auth/login", map[string]string{"Origin": "https://evil.invalid"}, 403}, {"POST", "/api/v2/auth/login", map[string]string{"X-Spotter-Request": ""}, 403}, {"GET", "/api/v2/workspace", nil, 401}, {"POST", "/api/workspace", nil, 409}, {"GET", "/day", nil, 200}, {"GET", "/missing.js", nil, 404}, {"POST", "/day", nil, 405}} {
		w := call(h, tc.method, tc.path, []byte(`{}`), tc.headers)
		if w.Code != tc.status {
			t.Errorf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("GET", "http://evil.invalid/livez", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, tc := range []struct {
		raw, content string
		status       int
	}{{`{"password":"x","password":"y"}`, "application/json", 400}, {`{}`, "text/plain", 415}, {strings.Repeat("x", (1<<20)+1), "application/json", 413}, {`{"password":"short"}`, "application/json", 422}} {
		w := call(h, "POST", "/api/v2/auth/setup", []byte(tc.raw), map[string]string{"Content-Type": tc.content})
		if w.Code != tc.status {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestAuthLifecycle(t *testing.T) {
	s, m := mockStore(t)
	a := API{Store: s, Origin: "http://localhost:18080"}
	m.ExpectQuery("SELECT EXISTS").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	w := call(a.Handler(), "GET", "/api/v2/auth/session", nil, nil)
	if !strings.Contains(w.Body.String(), `"setupRequired":true`) {
		t.Fatal(w.Body.String())
	}
	m.ExpectExec("INSERT INTO owner").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	m.ExpectExec("INSERT INTO sessions").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	w = call(a.Handler(), "POST", "/api/v2/auth/setup", []byte(`{"password":"unit-test-password"}`), nil)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 || !w.Result().Cookies()[0].HttpOnly {
		t.Fatal(w.Code)
	}
	cookie := w.Result().Cookies()[0]
	stored, _ := bcrypt.GenerateFromPassword([]byte("unit-test-password"), bcrypt.MinCost)
	m.ExpectQuery("SELECT password_hash").WillReturnRows(pgxmock.NewRows([]string{"password_hash"}).AddRow(string(stored)))
	m.ExpectExec("INSERT INTO sessions").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	w = call(a.Handler(), "POST", "/api/v2/auth/login", []byte(`{"password":"unit-test-password"}`), nil)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	m.ExpectExec("DELETE FROM sessions").WithArgs(hash([]byte(cookie.Value))).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	w = call(a.Handler(), "POST", "/api/v2/auth/logout", []byte(`{}`), map[string]string{"Cookie": cookie.String()})
	if w.Code != 200 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal(w.Code)
	}
	m.ExpectExec("INSERT INTO owner").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 0))
	w = call(a.Handler(), "POST", "/api/v2/auth/setup", []byte(`{"password":"unit-test-password"}`), nil)
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
}
func TestReadProjections(t *testing.T) {
	s, m := mockStore(t)
	a := API{Store: s}
	state := core.NewState()
	state.Tasks["t"] = core.Object{"id": "t", "title": "Title", "lifecycleState": "done"}
	state.Plans["p"] = core.Object{"id": "p", "date": "2026-10-05", "status": "active"}
	for _, path := range []string{"workspace", "capabilities", "tasks", "tasks/t", "tasks/missing", "inbox", "projects", "plans", "settings", "migration", "workday?date=2026-10-05", "wellbeing", "wellbeing/2026-10-05", "reports", "unknown"} {
		m.ExpectQuery("SELECT state").WillReturnRows(pgxmock.NewRows([]string{"state"}).AddRow(raw(state)))
		m.ExpectExec("INSERT INTO read_snapshots").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		w := call(http.HandlerFunc(a.read), "GET", "/api/v2/"+path, nil, nil)
		if w.Code != 200 && path != "unknown" && path != "tasks/missing" {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
}
func TestBatchValidation(t *testing.T) {
	b := Batch{SchemaVersion: 1, BatchID: "one", Source: "mail", Sequence: 1, ObservedAt: time.Now(), OK: true}
	if err := ValidateBatch(b); err != nil {
		t.Fatal(err)
	}
	b.Data.Mail = make([]model.MailMessage, 101)
	if err := ValidateBatch(b); err != nil {
		t.Fatal("ordinary source snapshot exceeding 100 records rejected", err)
	}
	for _, bad := range []Batch{{}, {SchemaVersion: 1, BatchID: "one", Source: "bad", Sequence: 1, ObservedAt: time.Now()}, {SchemaVersion: 1, BatchID: "one", Source: "mail", Sequence: 1, ObservedAt: time.Now(), Data: model.SourceData{Notes: []model.Note{{Title: "foreign"}}}}, {SchemaVersion: 1, BatchID: "one", Source: "mail", Sequence: 1, ObservedAt: time.Now(), Data: model.SourceData{Mail: make([]model.MailMessage, 10001)}}} {
		if ValidateBatch(bad) == nil {
			t.Fatal("invalid accepted")
		}
	}
}

func TestDeviceAndConsentHandlers(t *testing.T) {
	s, m := mockStore(t)
	a := API{Store: s}
	ctx := context.Background()
	_ = ctx
	m.ExpectExec("INSERT INTO enrollments").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	w := call(http.HandlerFunc(a.createEnrollment), "POST", "/api/v2/device-enrollments", nil, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	m.ExpectBegin()
	m.ExpectExec("DELETE FROM enrollments").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	m.ExpectExec("INSERT INTO devices").WithArgs(pgxmock.AnyArg(), "Mac", pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	m.ExpectCommit()
	m.ExpectRollback()
	w = call(http.HandlerFunc(a.enroll), "POST", "/api/v2/devices/enroll", []byte(`{"name":"Mac","code":"one"}`), nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	m.ExpectBegin()
	m.ExpectExec("DELETE FROM enrollments").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("DELETE", 0))
	m.ExpectRollback()
	w = call(http.HandlerFunc(a.enroll), "POST", "/api/v2/devices/enroll", []byte(`{"name":"Mac","code":"one"}`), nil)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	m.ExpectQuery("SELECT id,name,active").WillReturnRows(pgxmock.NewRows([]string{"id", "name", "active", "created_at", "last_seen"}).AddRow("mac", "Mac", true, time.Now(), nil))
	w = call(http.HandlerFunc(a.devices), "GET", "/api/v2/devices", nil, nil)
	if !strings.Contains(w.Body.String(), "Mac") {
		t.Fatal(w.Body.String())
	}
	m.ExpectExec("UPDATE devices").WithArgs("").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	w = call(http.HandlerFunc(a.revokeDevice), "POST", "/revoke", nil, nil)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	m.ExpectQuery("SELECT category,enabled").WillReturnRows(pgxmock.NewRows([]string{"category", "enabled", "generation"}).AddRow("diary", false, int64(1)))
	w = call(http.HandlerFunc(a.consents), "GET", "/consents", nil, nil)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	m.ExpectExec("UPDATE consents").WithArgs("diary", true).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	w = call(http.HandlerFunc(a.saveConsent), "POST", "/consents", []byte(`{"category":"diary","enabled":true}`), nil)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = call(http.HandlerFunc(a.saveConsent), "POST", "/consents", []byte(`{"category":"unknown","enabled":true}`), nil)
	if w.Code != 422 {
		t.Fatal(w.Code)
	}
	m.ExpectQuery("SELECT id FROM devices").WithArgs(hash([]byte("token"))).WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("mac"))
	w = call(a.deviceAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(deviceKey{}) != "mac" {
			t.Fatal("missing identity")
		}
		w.WriteHeader(204)
	})), "GET", "/x", nil, map[string]string{"Authorization": "Bearer token"})
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	m.ExpectQuery("SELECT EXISTS").WithArgs(hash([]byte("session"))).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	m.ExpectExec("UPDATE sessions").WithArgs(hash([]byte("session"))).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	w = call(a.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })), "GET", "/x", nil, map[string]string{"Cookie": "spotter_session=session"})
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
}
func TestIngestionBoundary(t *testing.T) {
	s, m := mockStore(t)
	a := API{Store: s}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.ingestion(w, r.WithContext(context.WithValue(r.Context(), deviceKey{}, "mac")))
	})
	m.ExpectQuery("SELECT state,error").WithArgs("batch", "mac").WillReturnRows(pgxmock.NewRows([]string{"state", "error", "applied_at"}).AddRow("applied", nil, nil))
	w := call(handler, "GET", "/api/v2/integrations/batches/batch", nil, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	m.ExpectQuery("SELECT state,error").WithArgs("unknown", "mac").WillReturnError(pgx.ErrNoRows)
	w = call(handler, "GET", "/api/v2/integrations/batches/unknown", nil, nil)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	b := Batch{SchemaVersion: 1, BatchID: "batch", Source: "notes", Sequence: 1, ObservedAt: time.Now()}
	bytes := raw(b)
	for _, state := range []string{"applied", "rejected", "pending"} {
		m.ExpectExec("INSERT INTO batches").WithArgs("batch", "mac", hash(bytes)).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		m.ExpectQuery("SELECT hash,device_id,state").WithArgs("batch").WillReturnRows(pgxmock.NewRows([]string{"hash", "device_id", "state"}).AddRow(hash(bytes), "mac", state))
		w = call(handler, "POST", "/api/v2/integrations/batches", bytes, map[string]string{"Idempotency-Key": "batch"})
		want := 200
		if state == "pending" {
			want = 503
		}
		if w.Code != want {
			t.Fatal(w.Body.String())
		}
	}
	m.ExpectExec("INSERT INTO batches").WithArgs("batch", "mac", hash(bytes)).WillReturnResult(pgxmock.NewResult("INSERT", 0))
	m.ExpectQuery("SELECT hash,device_id,state").WithArgs("batch").WillReturnRows(pgxmock.NewRows([]string{"hash", "device_id", "state"}).AddRow("changed", "mac", "pending"))
	w = call(handler, "POST", "/api/v2/integrations/batches", bytes, map[string]string{"Idempotency-Key": "batch"})
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
	w = call(handler, "POST", "/api/v2/integrations/batches", bytes, nil)
	if w.Code != 422 {
		t.Fatal(w.Code)
	}
	w = call(handler, "DELETE", "/x", nil, nil)
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
	m.ExpectQuery("SELECT device_id,name,snapshot").WillReturnRows(pgxmock.NewRows([]string{"device_id", "name", "snapshot", "updated_at"}).AddRow("mac", "notes", bytes, time.Now()))
	w = call(http.HandlerFunc(a.sources), "GET", "/sources", nil, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
}
func TestWorkerRevalidationAndReplay(t *testing.T) {
	s, m := mockStore(t)
	b := Batch{SchemaVersion: 1, BatchID: "batch", Source: "notes", Sequence: 1, ObservedAt: time.Now(), OK: true, Data: model.SourceData{Notes: []model.Note{{Title: "Fixture", Folder: "Inbox"}}}}
	q := queuedBatch{DeviceID: "mac", Batch: b}
	for _, tc := range []struct {
		active bool
		state  string
	}{{true, "pending"}, {true, "applied"}, {false, "pending"}} {
		m.ExpectBegin()
		m.ExpectQuery("SELECT active").WithArgs("mac").WillReturnRows(pgxmock.NewRows([]string{"active"}).AddRow(tc.active))
		m.ExpectQuery("SELECT state,hash").WithArgs("batch", "mac").WillReturnRows(pgxmock.NewRows([]string{"state", "hash"}).AddRow(tc.state, hash(raw(b))))
		if tc.state == "applied" {
			m.ExpectRollback()
		} else if !tc.active {
			m.ExpectExec("UPDATE batches").WithArgs("batch").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			m.ExpectCommit()
			m.ExpectRollback()
		} else {
			m.ExpectQuery("SELECT sequence").WithArgs("mac", "notes").WillReturnError(pgx.ErrNoRows)
			m.ExpectQuery("SELECT state").WillReturnRows(pgxmock.NewRows([]string{"state"}).AddRow(raw(core.NewState())))
			m.ExpectExec("UPDATE workspace").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			m.ExpectExec("INSERT INTO sources").WithArgs("mac", "notes", int64(1), raw(b)).WillReturnResult(pgxmock.NewResult("INSERT", 1))
			m.ExpectExec("UPDATE batches").WithArgs("batch").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			m.ExpectExec("UPDATE devices").WithArgs("mac").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			m.ExpectCommit()
			m.ExpectRollback()
		}
		if err := s.ApplyBatch(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
}
func TestStorageFailureResponses(t *testing.T) {
	s, _ := mockStore(t)
	a := API{Store: s}
	for _, handler := range []http.HandlerFunc{a.session, a.createEnrollment, a.devices, a.consents, a.sources, a.revokeDevice} {
		w := call(handler, "GET", "/x", nil, nil)
		if w.Code != 503 {
			t.Fatal(w.Code)
		}
	}
	for _, handler := range []http.HandlerFunc{a.setup, a.login, a.enroll, a.saveConsent, a.command} {
		w := call(handler, "POST", "/x", []byte("invalid"), nil)
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	if _, err := s.Read(context.Background()); err == nil {
		t.Fatal("read failure")
	}
	if err := s.Migrate(context.Background()); err == nil {
		t.Fatal("begin failure")
	}
	if _, err := s.Execute(context.Background(), core.Command{}); err == nil {
		t.Fatal("begin failure")
	}
	if _, _, err := s.Snapshot(context.Background(), ""); err == nil {
		t.Fatal("snapshot failure")
	}
	if _, err := s.ImportLegacy(context.Background(), []byte("bad"), true); err == nil {
		t.Fatal("legacy parse")
	}
	w := call(http.HandlerFunc(a.upload), "POST", "/files", nil, nil)
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	w = call(http.HandlerFunc(a.download), "GET", "/files/x", nil, nil)
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	a.Files = &Files{}
	w = call(http.HandlerFunc(a.upload), "POST", "/files", nil, nil)
	if w.Code != 422 {
		t.Fatal(w.Code)
	}
	w = call(http.HandlerFunc(a.download), "GET", "/files/x", nil, nil)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}

func TestFileRoundTripWithS3Adapter(t *testing.T) {
	content := []byte("unit object")
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			w.Header().Set("ETag", `"fixture"`)
			w.WriteHeader(200)
			return
		}
		if strings.Contains(r.URL.RawQuery, "location") {
			_, _ = w.Write([]byte(`<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`))
			return
		}
		w.Header().Set("Content-Length", "11")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", `"fixture"`)
		if r.Method == "GET" {
			_, _ = w.Write(content)
		}
	}))
	defer s3.Close()
	files, err := OpenFiles(strings.TrimPrefix(s3.URL, "http://"), "unit", "unit-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err = files.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	s, m := mockStore(t)
	a := API{Store: s, Files: files}
	m.ExpectExec("INSERT INTO files").WithArgs(pgxmock.AnyArg(), "unit.txt", int64(len(content)), hash(content)).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	m.ExpectExec("UPDATE files").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	w := call(http.HandlerFunc(a.upload), "POST", "/files", content, map[string]string{"X-File-Name": "unit.txt", "X-Content-SHA256": hash(content)})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	m.ExpectQuery("SELECT name,size,checksum").WithArgs("x").WillReturnRows(pgxmock.NewRows([]string{"name", "size", "checksum"}).AddRow("unit.txt", int64(len(content)), hash(content)))
	w = call(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.SetPathValue("id", "x"); a.download(w, r) }), "GET", "/files/x", nil, nil)
	if w.Code != 200 || w.Body.String() != string(content) {
		t.Fatal(w.Code, w.Body.String())
	}
	m.ExpectExec("INSERT INTO files").WithArgs(pgxmock.AnyArg(), "bad.txt", int64(len(content)), strings.Repeat("0", 64)).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	w = call(http.HandlerFunc(a.upload), "POST", "/files", content, map[string]string{"X-File-Name": "bad.txt", "X-Content-SHA256": strings.Repeat("0", 64)})
	if w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestCommandConsentAndReadReceipt(t *testing.T) {
	s, m := mockStore(t)
	a := API{Store: s}
	m.ExpectQuery("SELECT enabled").WillReturnRows(pgxmock.NewRows([]string{"enabled"}).AddRow(false))
	w := call(http.HandlerFunc(a.command), "POST", "/commands", raw(core.Command{OperationID: "diary", Command: "wellbeing.save"}), nil)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	state := core.NewState()
	m.ExpectQuery("SELECT state").WillReturnRows(pgxmock.NewRows([]string{"state"}).AddRow(raw(state)))
	m.ExpectExec("INSERT INTO read_snapshots").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	m.ExpectQuery("SELECT receipt").WithArgs("op").WillReturnRows(pgxmock.NewRows([]string{"receipt"}).AddRow([]byte(`{"operationId":"op"}`)))
	w = call(http.HandlerFunc(a.read), "GET", "/api/v2/operations/op", nil, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
}
