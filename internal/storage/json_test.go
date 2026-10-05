package storage

import (
	"context"
	"os"
	"path/filepath"
	"spotter/internal/model"
	"testing"
)

func TestJSONStore(t *testing.T) {
	ctx := context.Background()
	s := JSONStore{Path: filepath.Join(t.TempDir(), "sub", "state.json")}
	if _, e := s.Load(ctx); e != nil {
		t.Fatal(e)
	}
	state := model.AppState{Plan: model.DailyPlan{Summary: "saved"}}
	if e := s.Save(ctx, state); e != nil {
		t.Fatal(e)
	}
	got, e := s.Load(ctx)
	if e != nil || got.Plan.Summary != "saved" {
		t.Fatal(got, e)
	}
	_ = os.WriteFile(s.Path, []byte("bad"), 0600)
	if _, e = s.Load(ctx); e == nil {
		t.Fatal("corrupt accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if e = s.Save(cancelled, state); e == nil {
		t.Fatal("cancelled save")
	}
	if _, e = s.Load(cancelled); e == nil {
		t.Fatal("cancelled load")
	}
	s.Path = filepath.Dir(s.Path)
	if _, e = s.Load(ctx); e == nil {
		t.Fatal("directory read")
	}
	if e = s.Save(ctx, state); e == nil {
		t.Fatal("directory write")
	}
	s.Path = filepath.Join(s.Path, "state.json", "x")
	if e = s.Save(ctx, state); e == nil {
		t.Fatal("bad parent")
	}
}
