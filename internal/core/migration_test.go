package core

import (
	"encoding/json"
	"spotter/internal/workspace"
	"testing"
)

func TestLegacyIdentityAndUnknownHistory(t *testing.T) {
	old := workspace.Data{Version: 4, Settings: workspace.Settings{Zone: "Europe/Moscow", Start: "09:30", End: "18:00", Reserve: 25, Block: 90, Break: 15, SleepTarget: 8}}
	for _, status := range []string{"done", "doing", "waiting", "ready", "deferred", "inbox"} {
		old.Tasks = append(old.Tasks, workspace.Task{ID: status, Title: status, Project: "Same", Status: status, Minutes: 90})
	}
	b, _ := json.Marshal(old)
	s, e := MigrateLegacy(b)
	if e != nil || len(s.Tasks) != 6 || len(s.Projects) != 1 {
		t.Fatal(s, e)
	}
	if s.Tasks["done"]["completionEventId"] != nil || s.Tasks["done"]["createdAt"] != nil {
		t.Fatal("fabricated history")
	}
	old.Tasks[0].Status = "bad"
	b, _ = json.Marshal(old)
	if _, e = MigrateLegacy(b); e == nil {
		t.Fatal("unknown status")
	}
	old.Tasks[0].ID = ""
	b, _ = json.Marshal(old)
	if _, e = MigrateLegacy(b); e == nil {
		t.Fatal("missing ID")
	}
	if _, e = MigrateLegacy([]byte(`{}`)); e == nil {
		t.Fatal("empty settings")
	}
}
