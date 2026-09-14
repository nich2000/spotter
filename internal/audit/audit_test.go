package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"spotter/internal/model"
	"spotter/internal/planner"
)

func TestJSONLStoreAppendsRefreshRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "refresh_log.jsonl")
	store := &JSONLStore{Path: path}
	record := RefreshRecord{
		GeneratedAt: time.Date(2026, 6, 17, 16, 30, 0, 0, time.UTC),
		Sources: []SourceRecord{{
			Name: "mail",
			OK:   true,
			Data: model.SourceData{Mail: []model.MailMessage{{Subject: "hello", IsUnread: true}}},
		}},
		Model: planner.Trace{
			Model:    "qwen3:4b",
			Prompt:   `{"mail":1}`,
			Response: `{"summary":"ok","blocks":[],"risks":[],"focus":[]}`,
		},
		Plan: model.DailyPlan{Summary: "ok"},
	}

	if err := store.SaveRefresh(context.Background(), record); err != nil {
		t.Fatalf("SaveRefresh() error = %v", err)
	}
	if err := store.SaveRefresh(context.Background(), record); err != nil {
		t.Fatalf("second SaveRefresh() error = %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
		var got RefreshRecord
		if err := json.Unmarshal(scanner.Bytes(), &got); err != nil {
			t.Fatalf("line %d is not JSON: %v", count, err)
		}
		if got.Sources[0].Name != "mail" {
			t.Fatalf("source name = %q, want mail", got.Sources[0].Name)
		}
		if got.Model.Prompt == "" || got.Model.Response == "" {
			t.Fatalf("model trace not saved: %+v", got.Model)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan log: %v", err)
	}
	if count != 2 {
		t.Fatalf("line count = %d, want 2", count)
	}
}
