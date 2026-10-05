package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelperWithOSFixtures(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("SPOTTER_AGENT_DIR", filepath.Join(dir, "queue"))
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Error(err)
		}
	})
	_ = os.WriteFile("config.yaml", []byte("notes:\n  folder: Test\n"), 0600)
	security := `#!/bin/sh
case "$*" in
  *-queue*) printf '%064d' 0;;
  *find-generic*) echo fixture-token;;
  *) exit 0;;
esac
`
	if e := os.WriteFile(filepath.Join(dir, "security"), []byte(security), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "osascript"), []byte("#!/bin/sh\necho '[]'\n"), 0700); e != nil {
		t.Fatal(e)
	}
	received := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/devices/enroll":
			_, _ = io.WriteString(w, `{"deviceId":"fixture","token":"fixture-token"}`)
		case "/api/v2/integrations/consents":
			_, _ = io.WriteString(w, `{"enabled":false,"generation":1}`)
		default:
			if r.Method == "POST" {
				received++
			}
			_, _ = io.WriteString(w, `{"state":"applied"}`)
		}
	}))
	defer srv.Close()
	if err := run(context.Background(), srv.URL, "one-use-code", true); err != nil {
		t.Fatal(err)
	}
	if received != 4 {
		t.Fatal("missing collectors", received)
	}
	if err := run(context.Background(), srv.URL, "", true); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"://bad", "http://example.com", "https://example.com/path", "https://user@example.com"} {
		if run(context.Background(), bad, "", true) == nil {
			t.Fatal("invalid server accepted", bad)
		}
	}
	if err := saveKey(context.Background(), "test", "fixture"); err != nil {
		t.Fatal(err)
	}
	value, err := keychain(context.Background(), "test")
	if err != nil || strings.TrimSpace(value) != "fixture-token" {
		t.Fatal(value, err)
	}
}
