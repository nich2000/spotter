package script

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunner(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	path := filepath.Join(dir, "osascript")
	write := func(body string) {
		t.Helper()
		if e := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0700); e != nil {
			t.Fatal(e)
		}
	}
	write("printf '%s' \"$1\"")
	out, e := (Runner{}).Run(context.Background(), "test")
	if e != nil || string(out) != "test" {
		t.Fatal(string(out), e)
	}
	write("echo denied >&2; exit 1")
	if _, e = (Runner{}).Run(context.Background(), "test"); e == nil || !strings.Contains(e.Error(), "denied") {
		t.Fatal(e)
	}
	write("exit 1")
	if _, e = (Runner{}).Run(context.Background(), "test"); e == nil {
		t.Fatal("exit")
	}
	write("exec sleep 1")
	if _, e = (Runner{Timeout: time.Millisecond}).Run(context.Background(), "test"); e == nil || !strings.Contains(e.Error(), "timeout") {
		t.Fatal(e)
	}
}
