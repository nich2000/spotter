package calendar

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectAdapter(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	path := filepath.Join(dir, "osascript")
	c := Collector{ScriptPath: "fixture"}
	if c.Name() != "calendar" {
		t.Fatal(c.Name())
	}
	for _, tc := range []struct {
		body    string
		wantErr bool
	}{{"echo '[]'", false}, {"echo 'not json'", true}, {"exit 1", true}} {
		if e := os.WriteFile(path, []byte("#!/bin/sh\n"+tc.body), 0700); e != nil {
			t.Fatal(e)
		}
		_, e := c.Collect(context.Background())
		if (e != nil) != tc.wantErr {
			t.Fatal(e)
		}
	}
}
