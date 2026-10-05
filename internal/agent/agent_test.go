package agent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"spotter/internal/platform"
	"strings"
	"testing"
	"time"
)

func TestEncryptedQueueDelivery(t *testing.T) {
	state := "received"
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-token" {
			t.Error("token")
		}
		if r.Method == "POST" {
			posts++
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), `"batchId":"batch"`) || r.Header.Get("Idempotency-Key") != "batch" {
				t.Error("identity")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":"` + state + `"}`))
	}))
	defer server.Close()
	a := Agent{URL: server.URL, Token: "private-token", Dir: t.TempDir(), Key: make([]byte, 32)}
	b := platform.Batch{SchemaVersion: 1, BatchID: "batch", Source: "notes", Sequence: 1, ObservedAt: time.Now(), OK: true}
	if err := a.Enqueue(b); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.Dir, "batch.batch")
	encrypted, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(encrypted), "notes") {
		t.Fatal("plaintext")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("permissions")
	}
	if err = a.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("removed on received")
	}
	state = "applied"
	if err = a.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) || posts != 2 {
		t.Fatal("terminal receipt", err, posts)
	}
	if err = a.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestQueueFailureRetention(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	a := Agent{URL: server.URL, Dir: t.TempDir(), Key: make([]byte, 32)}
	b := platform.Batch{SchemaVersion: 1, BatchID: "batch", Source: "mail", Sequence: 1, ObservedAt: time.Now()}
	if err := a.Enqueue(b); err != nil {
		t.Fatal(err)
	}
	if a.Flush(context.Background()) == nil {
		t.Fatal("expected offline error")
	}
	entries, _ := os.ReadDir(a.Dir)
	if len(entries) != 1 {
		t.Fatal("lost queue")
	}
	a.Key = []byte("short")
	if a.Enqueue(b) == nil {
		t.Fatal("bad key")
	}
	a.Key = make([]byte, 32)
	if _, err := a.open([]byte{1}); err == nil {
		t.Fatal("truncated")
	}
	sealed, _ := a.seal([]byte("private"))
	sealed[len(sealed)-1] ^= 1
	if _, err := a.open(sealed); err == nil {
		t.Fatal("tampered")
	}
	b.BatchID = "../bad"
	if a.Enqueue(b) == nil {
		t.Fatal("path traversal")
	}
	b.SchemaVersion = 2
	if a.Enqueue(b) == nil {
		t.Fatal("schema")
	}
	a.Dir = filepath.Join(t.TempDir(), "missing")
	if err := a.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHealthOptInAndValidation(t *testing.T) {
	enabled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if enabled {
			_, _ = w.Write([]byte(`{"enabled":true,"generation":3}`))
		} else {
			_, _ = w.Write([]byte(`{"enabled":false,"generation":2}`))
		}
	}))
	defer server.Close()
	a := Agent{URL: server.URL, Dir: t.TempDir(), Key: make([]byte, 32)}
	path := filepath.Join(t.TempDir(), "health.json")
	if err := a.CollectHealth(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(a.Dir)
	if len(entries) != 0 {
		t.Fatal("health read without consent")
	}
	enabled = true
	if err := a.CollectHealth(context.Background(), path); err != nil {
		t.Fatal("missing file should wait", err)
	}
	raw := `{"measuredAt":"` + time.Now().UTC().Format(time.RFC3339) + `","sleep":[]}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.CollectHealth(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	entries, _ = os.ReadDir(a.Dir)
	if len(entries) != 1 {
		t.Fatal("aggregate not queued")
	}
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if a.CollectHealth(context.Background(), path) == nil {
		t.Fatal("invalid health")
	}
	a.URL = "://bad"
	if _, err := a.request(context.Background(), "GET", "", nil, ""); err == nil {
		t.Fatal("bad URL")
	}
}
