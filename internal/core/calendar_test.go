package core

import (
	"reflect"
	"testing"
	"time"
)

func calendarFixture() (State, Object) {
	s := NewState()
	s.Settings["zone"] = "UTC"
	s.Settings["workStart"] = "09:00"
	s.Tasks["t"] = Object{"id": "t", "lifecycleState": "not_started", "estimateMinutes": 300}
	p := Object{"taskId": "t", "startAt": "2026-10-05T11:00:00Z", "minutes": 50, "zone": "UTC"}
	return s, p
}
func TestCalendarBlocks(t *testing.T) {
	s, p := calendarFixture()
	before := Copy(s)
	rows, err := ProposeBlocks(s, p, clock)
	if err != nil || len(rows) != 3 || str(rows[1], "kind") != "break" || str(rows[2], "startAt") != "2026-10-05T11:30:00Z" {
		t.Fatal(rows, err)
	}
	if !reflect.DeepEqual(before, Copy(s)) {
		t.Fatal("preview mutated state")
	}
	r := apply(t, &s, "workblocks.create", p)
	if len(s.WorkBlocks) != 3 || len(s.Plans) != 0 || len(s.Events) != 3 {
		t.Fatal(s)
	}
	id := r.Result["blockIds"].([]string)[0]
	apply(t, &s, "workblocks.pin", Object{"blockId": id, "pinned": true})
	if !flag(s.WorkBlocks[id], "pinned") {
		t.Fatal("pin")
	}
	apply(t, &s, "workblocks.delete", Object{"blockId": id})
	if len(s.WorkBlocks) != 2 || len(s.Tasks) != 1 {
		t.Fatal("delete changed task")
	}
	reject(t, s, "workblocks.pin", Object{"blockId": "missing"}, "ENTITY_NOT_FOUND")
	reject(t, s, "workblocks.unknown", Object{}, "UNKNOWN_COMMAND")
}
func TestCalendarValidation(t *testing.T) {
	for _, change := range []func(*State, Object){
		func(s *State, p Object) { p["unknown"] = true }, func(s *State, p Object) { p["taskId"] = "missing" }, func(s *State, p Object) { s.Tasks["t"]["lifecycleState"] = "done" }, func(s *State, p Object) { p["zone"] = "bad" }, func(s *State, p Object) { p["startAt"] = "bad" }, func(s *State, p Object) { p["minutes"] = 0 }, func(s *State, p Object) { p["minutes"] = 1.5 }, func(s *State, p Object) { s.Tasks["t"]["estimateMinutes"] = nil }, func(s *State, p Object) { s.Tasks["t"]["remainingMinutes"] = 10 }, func(s *State, p Object) { p["startAt"] = "2026-10-10T10:00:00Z" }, func(s *State, p Object) { p["startAt"] = "2026-10-05T08:00:00Z" }, func(s *State, p Object) { p["startAt"] = "2026-10-05T17:30:00Z" }, func(s *State, p Object) {
			s.Tasks["t"]["scheduled"] = Object{"kind": "date_range", "startDate": "2026-10-06", "zone": "UTC"}
		}, func(s *State, p Object) {
			s.Tasks["t"]["scheduled"] = Object{"kind": "date_range", "startDate": "2026-10-01", "endDate": "2026-10-04", "zone": "UTC"}
		}, func(s *State, p Object) {
			s.Tasks["t"]["deadline"] = Object{"kind": "date", "date": "2026-10-04", "zone": "UTC"}
		}, func(s *State, p Object) {
			s.Tasks["t"]["deadline"] = Object{"kind": "instant", "at": "2026-10-05T11:10:00Z", "zone": "UTC"}
		}, func(s *State, p Object) { p["automatic"] = true }, func(s *State, p Object) { s.Settings["workStart"] = "bad" },
	} {
		s, p := calendarFixture()
		change(&s, p)
		if _, err := ProposeBlocks(s, p, clock); err == nil {
			t.Fatal("accepted invalid", p, s)
		}
	}
}
func TestCalendarAvailability(t *testing.T) {
	s, p := calendarFixture()
	source := Object{"id": "cal", "ok": true, "observedAt": clock.Format(time.RFC3339), "from": "2026-10-01T00:00:00Z", "to": "2026-11-01T00:00:00Z", "events": []any{Object{"startAt": "2026-10-05T10:00:00Z", "endAt": "2026-10-05T12:00:00Z"}}}
	s.CalendarSources = map[string]Object{"cal": source}
	if _, err := ProposeBlocks(s, p, clock); err == nil {
		t.Fatal("conflict")
	}
	p["automatic"] = true
	rows, err := ProposeBlocks(s, p, clock)
	if err != nil || str(rows[0], "startAt") != "2026-10-05T12:00:00Z" {
		t.Fatal(rows, err)
	}
	p["automatic"] = false
	for _, key := range []string{"cancelled", "allDay", "availability"} {
		e := source["events"].([]any)[0].(Object)
		e[key] = true
		if key == "availability" {
			e[key] = "free"
		}
		if _, err := ProposeBlocks(s, p, clock); err != nil {
			t.Fatal(key, err)
		}
		delete(e, key)
	}
	source["ok"] = false
	if _, err := ProposeBlocks(s, p, clock); err == nil {
		t.Fatal("stale")
	}
	source["ok"] = true
	source["observedAt"] = "2026-10-01T00:00:00Z"
	if _, err := ProposeBlocks(s, p, clock); err == nil {
		t.Fatal("old")
	}
	source["observedAt"] = clock.Format(time.RFC3339)
	source["from"] = "2026-10-06T00:00:00Z"
	if _, err := ProposeBlocks(s, p, clock); err == nil {
		t.Fatal("coverage")
	}
}
func TestCalendarReserveAndSplitting(t *testing.T) {
	s, p := calendarFixture()
	p["minutes"] = 125
	rows, err := ProposeBlocks(s, p, clock)
	if err != nil || len(rows) != 9 || at(rows[7], "endAt").Sub(at(rows[7], "startAt")) != 15*time.Minute {
		t.Fatal(rows, err)
	}
	s.Tasks["t"]["splittable"] = false
	rows, err = ProposeBlocks(s, p, clock)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	s.Tasks["t"]["estimateMinutes"] = 1000
	p["minutes"] = 500
	p["startAt"] = "2026-10-05T09:00:00Z"
	if _, err := ProposeBlocks(s, p, clock); err == nil {
		t.Fatal("reserve")
	}
	p["minutes"] = 25
	apply(t, &s, "workblocks.create", p)
	p["minutes"] = 50
	if _, err := ProposeBlocks(s, p, clock); err == nil {
		t.Fatal("existing collision")
	}
	s.Tasks["t"]["remainingMinutes"] = 20
	p["startAt"] = "2026-10-05T13:00:00Z"
	if _, err := ProposeBlocks(s, p, clock); err == nil {
		t.Fatal("remaining")
	}
}
func TestCalendarWorkSettings(t *testing.T) {
	s, _ := calendarFixture()
	for _, patch := range []Object{{"pomodoroMinutes": 0}, {"pomodoroBreakMinutes": 121}, {"pomodoroLongBreakMinutes": 2.5}, {"workWeek": "bad"}, {"workWeek": Object{"7": nil}}, {"workWeek": Object{"1": "bad"}}, {"workWeek": Object{"1": Object{"start": "10:00", "end": "09:00"}}}, {"workExceptions": Object{"bad": nil}}} {
		if validateCalendarSettings(patch) == nil {
			t.Fatal(patch)
		}
	}
	s.Settings["workWeek"] = Object{"1": nil}
	_, _, err := workWindow(s, "2026-10-05", time.UTC)
	if err == nil {
		t.Fatal("off")
	}
	s.Settings["workExceptions"] = Object{"2026-10-05": Object{"start": "10:00", "end": "16:00"}}
	if validateCalendarSettings(s.Settings) != nil {
		t.Fatal("valid settings")
	}
	a, b, err := workWindow(s, "2026-10-05", time.UTC)
	if err != nil || b.Sub(a) != 6*time.Hour {
		t.Fatal(a, b, err)
	}
	if _, _, err := workWindow(s, "bad", time.UTC); err == nil {
		t.Fatal("date")
	}
}
func TestCalendarDistribution(t *testing.T) {
	s, _ := calendarFixture()
	s.CalendarSources = map[string]Object{"cal": {"ok": true, "observedAt": clock.Format(time.RFC3339), "from": "2026-10-01T00:00:00Z", "to": "2026-11-01T00:00:00Z", "events": []any{}}}
	s.Tasks["t"]["estimateMinutes"] = 500
	s.Tasks["t"]["title"] = "Long task"
	p := Object{"taskIds": []any{"t"}, "fromDate": "2026-10-05", "toDate": "2026-10-06", "zone": "UTC"}
	rows, err := ProposeBlocks(s, p, clock)
	if err != nil || len(rows) < 20 {
		t.Fatal(len(rows), err)
	}
	total := 0.0
	for _, r := range rows {
		if str(r, "kind") == "work" {
			total += at(r, "endAt").Sub(at(r, "startAt")).Minutes()
		}
	}
	if total != 500 {
		t.Fatal(total)
	}
	p["toDate"] = "2026-10-05"
	if _, err := ProposeBlocks(s, p, clock); err == nil {
		t.Fatal("deficit hidden")
	}
	p["taskIds"] = []any{"t", "t"}
	if _, err := ProposeBlocks(s, p, clock); err == nil {
		t.Fatal("duplicate")
	}
	p["taskIds"] = []any{}
	if _, err := ProposeBlocks(s, p, clock); err == nil {
		t.Fatal("empty")
	}
	p["fromDate"] = "bad"
	if _, err := ProposeBlocks(s, p, clock); err == nil {
		t.Fatal("bad date")
	}
}
