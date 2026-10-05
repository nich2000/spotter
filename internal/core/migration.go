package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"spotter/internal/workspace"
	"strings"
)

// MigrateLegacy preserves IDs and unknown historical timestamps. The caller keeps the original bytes as backup.
func MigrateLegacy(raw []byte) (State, error) {
	var old workspace.Data
	if err := Decode(raw, &old); err != nil {
		return State{}, err
	}
	if old.Settings.Zone == "" {
		return State{}, invalid("Legacy settings missing")
	}
	s := NewState()
	s.Settings = Object{"zone": old.Settings.Zone, "workStart": old.Settings.Start, "workEnd": old.Settings.End, "reservePercent": old.Settings.Reserve, "focusBlockMinutes": old.Settings.Block, "scheduleBreakMinutes": old.Settings.Break, "plannerSleepTargetHours": old.Settings.SleepTarget}
	sum := sha256.Sum256(raw)
	s.Migration = Object{"status": "imported", "sourceSHA256": hex.EncodeToString(sum[:]), "historyCompleteness": "partial", "baseline": "legacy", "legacyVersion": old.Version, "legacy": old}
	projectIDs := map[string]string{}
	for _, t := range old.Tasks {
		if t.ID == "" || strings.TrimSpace(t.Title) == "" || s.Tasks[t.ID] != nil {
			return State{}, invalid("Некорректная или повторная legacy задача")
		}
		var project any
		if t.Project != "" {
			pid := projectIDs[t.Project]
			if pid == "" {
				pid = ID()
				projectIDs[t.Project] = pid
				s.Projects[pid] = Object{"id": pid, "name": t.Project, "parentId": nil, "position": len(projectIDs) - 1}
			}
			project = pid
		}
		state := "not_started"
		var mode any
		switch t.Status {
		case "done":
			state = "done"
		case "doing":
			state = "in_progress"
			mode = "active"
		case "waiting":
			state = "in_progress"
			mode = "waiting"
		case "ready", "deferred", "inbox":
		default:
			return State{}, invalid(fmt.Sprintf("Неизвестное legacy состояние: %s", t.Status))
		}
		s.Tasks[t.ID] = Object{"id": t.ID, "definitionId": ID(), "title": t.Title, "description": "", "resume": t.Resume, "projectId": project, "context": "anywhere", "importance": "unknown", "urgency": "unknown", "reviewRequired": true, "complexity": nil, "multiDay": false, "estimateMinutes": t.Minutes, "remainingMinutes": nil, "lifecycleState": state, "workMode": mode, "completionEventId": nil, "createdAt": nil, "updatedAt": t.UpdatedAt, "subtasks": []any{}, "comments": []any{}, "legacy": t}
	}
	return s, nil
}
