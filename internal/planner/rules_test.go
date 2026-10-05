package planner

import (
	"context"
	"spotter/internal/model"
	"testing"
	"time"
)

func TestRuleBasedCompleteInputs(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	yesterday := now.Add(-24 * time.Hour)
	tomorrow := now.Add(24 * time.Hour)
	state := model.AppState{Calendar: []model.CalendarEvent{{Title: "Later", Start: now.Add(time.Hour)}, {Title: "Earlier", Start: now}}, Reminders: []model.Reminder{{Title: "Today", DueDate: &now}, {Title: "Late", DueDate: &yesterday}, {Title: "Future", DueDate: &tomorrow}, {Title: "No date"}}, Notes: []model.Note{{Title: "Note"}}}
	for i := 0; i < 12; i++ {
		state.Mail = append(state.Mail, model.MailMessage{IsUnread: true})
	}
	p := RuleBased{Now: func() time.Time { return now }}
	out, e := p.Generate(context.Background(), state)
	if e != nil || len(out.Risks) != 2 || len(out.Focus) != 3 || len(out.Blocks) != 3 {
		t.Fatal(out, e)
	}
	out, e = p.Generate(context.Background(), model.AppState{})
	if e != nil || len(out.Focus) != 1 {
		t.Fatal(out, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = p.Generate(ctx, state); e == nil {
		t.Fatal("cancel")
	}
	if _, trace, e := GenerateWithTrace(ctx, p, state); e == nil || trace.Status != "failed" {
		t.Fatal(trace, e)
	}
	if _, trace, e := GenerateWithTrace(context.Background(), p, state); e != nil || trace.Status != "ok" {
		t.Fatal(trace, e)
	}
}
