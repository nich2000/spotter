package core

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

var clock = time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)

func apply(t *testing.T, s *State, name string, p Object) Receipt {
	t.Helper()
	b, _ := json.Marshal(p)
	_ = json.Unmarshal(b, &p)
	next, r, err := Apply(*s, Command{OperationID: ID(), ExpectedRevision: s.Revision, Command: name, Payload: p}, clock)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	*s = next
	return r
}
func reject(t *testing.T, s State, name string, p Object, code string) {
	t.Helper()
	before := Copy(s)
	b, _ := json.Marshal(p)
	_ = json.Unmarshal(b, &p)
	next, _, err := Apply(s, Command{OperationID: ID(), ExpectedRevision: s.Revision, Command: name, Payload: p}, clock)
	var e *Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("%s: want %s got %v", name, code, err)
	}
	if !reflect.DeepEqual(Copy(next), before) || !reflect.DeepEqual(Copy(s), before) {
		t.Fatal("failed command modified state")
	}
}
func TestTaskAtomicLifecycle(t *testing.T) {
	s := NewState()
	p := apply(t, &s, "project.create", Object{"name": "Work", "parentId": nil})
	pid := p.Result["projectId"]
	r := apply(t, &s, "task.create", Object{"title": " Ship ", "projectId": pid, "patch": Object{"description": "Details", "resume": "Next", "estimateMinutes": 25, "remainingMinutes": 0, "complexity": "easy", "multiDay": true, "context": "computer", "importance": "yes", "urgency": "no", "priorityNote": "reason", "deadline": nil, "scheduled": nil, "inboxAvailableAt": nil}, "subtasks": []any{Object{"clientRef": "one", "title": "Verify", "done": false, "position": 0}}, "commentsToAdd": []any{Object{"clientRef": "note", "text": "First"}}})
	id := r.Result["taskId"].(string)
	if s.Tasks[id]["title"] != "Ship" || len(s.Events) != 2 {
		t.Fatal(s.Tasks[id])
	}
	reject(t, s, "task.save", Object{"taskId": id, "patch": Object{"title": "Lost"}, "planChanges": []any{Object{"planId": "missing", "action": "upsert"}}}, "ENTITY_NOT_FOUND")
	apply(t, &s, "task.triage", Object{"taskId": id, "triage": Object{"action": "complete", "importance": "yes", "urgency": "no"}})
	apply(t, &s, "task.triage", Object{"taskId": id, "triage": Object{"action": "require", "clearPriority": true}})
	for _, mode := range []string{"active", "paused", "waiting", "background"} {
		apply(t, &s, "task.transition", Object{"taskId": id, "transition": Object{"to": "in_progress", "workMode": mode, "waitingOn": "reply", "checkAt": Object{"kind": "date", "date": "2026-10-06", "zone": "Europe/Moscow"}}})
	}
	reject(t, s, "task.transition", Object{"taskId": id, "transition": Object{"to": "done", "completionActor": "self"}}, "UNFINISHED_SUBTASKS_DECISION_REQUIRED")
	apply(t, &s, "task.transition", Object{"taskId": id, "transition": Object{"to": "done", "completionActor": "other", "unfinishedSubtasksDecision": "complete_all"}})
	event := s.Tasks[id]["completionEventId"]
	reject(t, s, "task.reopen", Object{"taskId": id, "completionEventId": "old"}, "VERSION_CONFLICT")
	apply(t, &s, "task.reopen", Object{"taskId": id, "completionEventId": event})
	apply(t, &s, "task.move", Object{"taskId": id, "projectId": nil})
	r = apply(t, &s, "task.move", Object{"taskId": id, "projectId": nil})
	if r.Result["noChange"] != true {
		t.Fatal("same project is not a noop")
	}
	st := s.Tasks[id]["subtasks"].([]any)[0].(map[string]any)
	apply(t, &s, "task.save", Object{"taskId": id, "subtasks": []any{Object{"id": st["id"], "title": "Updated", "done": true, "position": 0}}})
	if s.Tasks[id]["remainingMinutes"] != float64(0) {
		t.Fatal("zero remaining completed task")
	}
}
func TestTaskValidation(t *testing.T) {
	s := NewState()
	r := apply(t, &s, "task.create", Object{"title": "Task"})
	id := r.Result["taskId"].(string)
	for _, patch := range []Object{{"unknown": true}, {"title": ""}, {"title": nil}, {"description": strings.Repeat("x", 20001)}, {"resume": strings.Repeat("x", 5001)}, {"projectId": "missing"}, {"projectId": 5}, {"context": "bad"}, {"importance": "bad"}, {"urgency": nil}, {"complexity": "bad"}, {"multiDay": "true"}, {"estimateMinutes": 0}, {"estimateMinutes": 1.5}, {"remainingMinutes": -1}, {"scheduled": Object{}}} {
		code := "VALIDATION_FAILED"
		if patch["unknown"] != nil {
			code = "UNKNOWN_FIELD"
		}
		if patch["projectId"] == "missing" {
			code = "ENTITY_NOT_FOUND"
		}
		reject(t, s, "task.save", Object{"taskId": id, "patch": patch}, code)
	}
	for _, p := range []Object{{"subtasks": "bad"}, {"subtasks": []any{"bad"}}, {"subtasks": []any{Object{"title": "", "done": false}}}, {"subtasks": []any{Object{"title": "OK"}}}, {"subtasks": []any{Object{"id": "foreign", "title": "OK", "done": false}}}, {"subtasks": []any{Object{"title": "OK", "done": false}}}, {"commentsToAdd": []any{"bad"}}, {"commentsToAdd": []any{Object{"clientRef": "a", "text": ""}}}, {"commentsToAdd": []any{Object{"text": "ok"}}}, {"triage": Object{"action": "complete", "importance": "yes"}}, {"triage": Object{"action": "bad"}}, {"transition": Object{"to": "bad"}}, {"transition": Object{"to": "in_progress", "workMode": "bad"}}, {"transition": Object{"to": "in_progress", "workMode": "waiting"}}, {"transition": Object{"to": "in_progress", "workMode": "waiting", "waitingOn": "why"}}, {"transition": Object{"to": "done"}}, {"transition": Object{"to": "done"}, "reopen": Object{}}, {"planChanges": []any{"bad"}}} {
		p["taskId"] = id
		reject(t, s, "task.save", p, "VALIDATION_FAILED")
	}
	reject(t, s, "task.save", Object{"taskId": id, "recurrenceChange": Object{}}, "POLICY_UNRESOLVED")
	reject(t, s, "task.save", Object{"taskId": id, "timeEntriesToAdd": []any{Object{}}}, "POLICY_UNRESOLVED")
	reject(t, s, "task.create", Object{"title": "A", "patch": Object{"title": "B"}}, "VALIDATION_FAILED")
	reject(t, s, "task.create", Object{"title": "A", "patch": Object{"projectId": "b"}}, "VALIDATION_FAILED")
	reject(t, s, "task.reopen", Object{"taskId": id}, "VALIDATION_FAILED")
	reject(t, s, "task.bad", Object{"taskId": id}, "UNKNOWN_COMMAND")
	reject(t, s, "task.save", Object{"taskId": "bad"}, "ENTITY_NOT_FOUND")
	reject(t, s, "unknown", Object{}, "UNKNOWN_COMMAND")
	for _, cmd := range []string{"timer.start", "focus.select", "time.add", "recurrence.create"} {
		reject(t, s, cmd, Object{}, "POLICY_UNRESOLVED")
	}
}
func TestProjectTree(t *testing.T) {
	s := NewState()
	a := apply(t, &s, "project.create", Object{"name": "A"}).Result["projectId"].(string)
	b := apply(t, &s, "project.create", Object{"name": "B", "parentId": a}).Result["projectId"].(string)
	apply(t, &s, "project.rename", Object{"projectId": b, "name": "C"})
	reject(t, s, "project.move", Object{"projectId": a, "parentId": b}, "PROJECT_CYCLE")
	reject(t, s, "project.create", Object{"name": "C", "parentId": a}, "POLICY_UNRESOLVED")
	apply(t, &s, "project.move", Object{"projectId": b, "parentId": nil})
	reject(t, s, "project.rename", Object{"projectId": b, "name": ""}, "VALIDATION_FAILED")
	reject(t, s, "project.no", Object{"projectId": b}, "UNKNOWN_COMMAND")
	reject(t, s, "project.move", Object{"projectId": "unknown"}, "ENTITY_NOT_FOUND")
	reject(t, s, "project.create", Object{"name": "X", "parentId": "unknown"}, "ENTITY_NOT_FOUND")
	for i := 0; i < 30; i++ {
		a = apply(t, &s, "project.create", Object{"name": "nested", "parentId": a}).Result["projectId"].(string)
	}
	a = apply(t, &s, "project.create", Object{"name": "nested", "parentId": a}).Result["projectId"].(string)
	reject(t, s, "project.create", Object{"name": "too deep", "parentId": a}, "VALIDATION_FAILED")
}
func TestPlansAndRollback(t *testing.T) {
	s := NewState()
	id := apply(t, &s, "task.create", Object{"title": "Task"}).Result["taskId"]
	p := Object{"date": "2026-10-05", "zone": "Europe/Moscow", "items": []any{Object{"taskId": id, "allocatedMinutes": nil, "position": 0}}, "mainOccurrenceId": id, "frogOccurrenceId": nil}
	r := apply(t, &s, "plan.draft.save", p)
	pid := r.Result["planId"]
	apply(t, &s, "plan.activate", Object{"planId": pid})
	reject(t, s, "plan.activate", Object{"planId": pid}, "PLAN_NOT_EDITABLE")
	reject(t, s, "plan.close", Object{"planId": pid}, "POLICY_UNRESOLVED")
	reject(t, s, "task.save", Object{"taskId": id, "planChanges": []any{Object{"planId": pid, "action": "remove"}}}, "VALIDATION_FAILED")
	apply(t, &s, "task.save", Object{"taskId": id, "planChanges": []any{Object{"planId": pid, "action": "remove", "clearMain": true}}})
	apply(t, &s, "task.save", Object{"taskId": id, "planChanges": []any{Object{"planId": pid, "action": "upsert", "allocatedMinutes": 30}}})
	r = apply(t, &s, "plan.draft.save", p)
	reject(t, s, "plan.activate", Object{"planId": r.Result["planId"]}, "ACTIVE_PLAN_EXISTS")
	apply(t, &s, "plan.draft.discard", Object{"planId": r.Result["planId"]})
	p["date"] = "2026-10-06"
	reject(t, s, "plan.draft.save", p, "POLICY_UNRESOLVED")
	p["date"] = "bad"
	reject(t, s, "plan.draft.save", p, "VALIDATION_FAILED")
	for _, change := range []Object{{"items": nil}, {"items": []any{"bad"}}, {"items": []any{Object{"taskId": "bad"}}}, {"items": []any{Object{"taskId": id}, Object{"taskId": id}}}, {"items": []any{Object{"taskId": id, "allocatedMinutes": 0}}}, {"items": []any{}, "mainOccurrenceId": id}, {"items": []any{Object{"taskId": id}}, "frogOccurrenceId": id}} {
		change["planId"] = pid
		code := "VALIDATION_FAILED"
		if rows, ok := change["items"].([]any); ok && len(rows) > 0 {
			if m, ok := rows[0].(Object); ok && m["taskId"] == "bad" {
				code = "ENTITY_NOT_FOUND"
			}
		}
		reject(t, s, "plan.update", change, code)
	}
}
func TestWellbeingAndSettings(t *testing.T) {
	s := NewState()
	apply(t, &s, "settings.update", Object{"patch": Object{"zone": "UTC", "workStart": "08:00", "workEnd": "17:00"}})
	for _, p := range []Object{{"zone": "bad"}, {"workStart": "bad"}, {"workEnd": "07:00"}, {"reservePercent": 0}, {"focusBlockMinutes": 0}} {
		reject(t, s, "settings.update", Object{"patch": p}, "VALIDATION_FAILED")
	}
	apply(t, &s, "wellbeing.save", Object{"date": "2026-10-05", "zone": "UTC", "note": "Hello", "wellbeing": Object{"mood": "unknown", "energy": "medium", "stress": "low", "confirm": true}, "habits": Object{"cigarettes": 0, "alcoholPortions": nil, "confirm": false}})
	day := s.Days["2026-10-05"]
	if day["wellbeingConfirmedAt"] == nil || day["habitsConfirmedAt"] != nil {
		t.Fatal(day)
	}
	reject(t, s, "wellbeing.save", Object{"date": "2026-10-05", "zone": "Europe/Moscow"}, "FIELD_IMMUTABLE")
	for _, p := range []Object{{"note": nil}, {"wellbeing": Object{"mood": "bad"}}, {"wellbeing": Object{"mood": "normal", "energy": "low", "stress": "high"}}, {"habits": Object{"cigarettes": -1}}, {"habits": Object{"cigarettes": 0}}} {
		p["date"] = "2026-10-05"
		p["zone"] = "UTC"
		reject(t, s, "wellbeing.save", p, "VALIDATION_FAILED")
	}
	reject(t, s, "wellbeing.save", Object{"date": "bad", "zone": "UTC"}, "VALIDATION_FAILED")
	reject(t, s, "wellbeing.save", Object{"date": "2026-10-05", "zone": "UTC", "measurementsToAdd": []any{Object{}}}, "POLICY_UNRESOLVED")
}
func TestCandidates(t *testing.T) {
	s := NewState()
	s.Candidates["c"] = Object{"id": "c", "candidateVersion": float64(1), "state": "pending", "proposedTitle": "Imported"}
	p := Object{"candidateId": "c", "candidateVersion": 1}
	apply(t, &s, "source.dismiss", p)
	apply(t, &s, "source.restore", p)
	p["card"] = Object{"patch": Object{"title": "Edited", "projectId": nil}}
	r := apply(t, &s, "source.accept", p)
	if len(s.Tasks) != 1 || s.Tasks[r.Result["taskId"].(string)]["title"] != "Edited" {
		t.Fatal(s)
	}
	reject(t, s, "source.accept", p, "CANDIDATE_ALREADY_ACCEPTED")
	p["candidateVersion"] = 2
	reject(t, s, "source.accept", p, "CANDIDATE_CHANGED")
}

func TestCandidateRejectsStaleSourceSnapshot(t *testing.T) {
	s := NewState()
	s.Candidates["c"] = Object{"id": "c", "candidateVersion": float64(1), "state": "pending", "proposedTitle": "Updated", "sourceSnapshotId": "current"}
	for _, command := range []string{"source.accept", "source.dismiss", "source.restore"} {
		reject(t, s, command, Object{"candidateId": "c", "candidateVersion": 1, "sourceSnapshotId": "old"}, "CANDIDATE_CHANGED")
	}
	apply(t, &s, "source.accept", Object{"candidateId": "c", "candidateVersion": 1, "sourceSnapshotId": "current"})
}

func TestDiscardCannotActivatePlan(t *testing.T) {
	s := NewState()
	r := apply(t, &s, "plan.draft.save", Object{"date": "2026-10-05", "zone": "UTC", "items": []any{}})
	id := r.Result["planId"].(string)
	reject(t, s, "plan.draft.discard", Object{"planId": id, "activate": true}, "VALIDATION_FAILED")
	apply(t, &s, "plan.draft.discard", Object{"planId": id})
	if s.Plans[id]["status"] != "discarded" {
		t.Fatal(s.Plans[id])
	}
}
func TestDecodeAndGuards(t *testing.T) {
	for _, raw := range []string{`{"a":1,"a":2}`, `{} {}`, `{`, `[1,`, `{"a":]}`, `{"a":{"x":1,"x":2}}`} {
		var v any
		if Decode([]byte(raw), &v) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	var v Object
	if err := Decode([]byte(`{"x":[1,{"y":null}]}`), &v); err != nil {
		t.Fatal(err)
	}
	var c Command
	if Decode([]byte(`{"unexpected":true}`), &c) == nil {
		t.Fatal("unknown field")
	}
	s := NewState()
	for _, c := range []Command{{}, {OperationID: "a", ExpectedRevision: 4}, {OperationID: strings.Repeat("a", 129)}} {
		if _, _, err := Apply(s, c, clock); err == nil {
			t.Fatal("bad command")
		}
	}
	s.SchemaVersion = 3
	if _, _, err := Apply(s, Command{}, clock); err == nil {
		t.Fatal("schema")
	}
	s.SchemaVersion = 2
	s.Revision = 9007199254740991
	if _, _, err := Apply(s, Command{OperationID: "a", ExpectedRevision: s.Revision, Command: "task.create", Payload: Object{"title": "x"}}, clock); err == nil {
		t.Fatal("overflow")
	}
}
