package core

import (
	"reflect"
	"testing"
	"time"
)

func TestScheduleValidation(t *testing.T) {
	valid := []any{nil, Object{"kind": "date_range", "startDate": "2026-10-05", "endDate": "2026-10-05", "zone": "Europe/Moscow"}, Object{"kind": "date_range", "startDate": "2026-10-05", "endDate": nil, "zone": "UTC"}, Object{"startAt": "2026-10-05T10:00:00Z", "endAt": nil, "zone": "Europe/Moscow", "localStart": "2026-10-05T13:00:00"}, Object{"kind": "timed", "startAt": "2026-10-25T01:30:00Z", "endAt": "2026-10-25T02:30:00Z", "zone": "Europe/Berlin", "localStart": "2026-10-25T02:30:00", "localEnd": "2026-10-25T03:30:00", "offsetChoice": "later"}}
	for _, v := range valid {
		if e := validateSchedule(v); e != nil {
			t.Fatalf("%v: %v", v, e)
		}
	}
	bad := []any{"bad", Object{}, Object{"kind": "other", "zone": "UTC"}, Object{"kind": "date_range", "zone": "UTC", "startDate": "2026-02-30"}, Object{"kind": "date_range", "zone": "UTC", "startDate": "2026-10-05", "endDate": "2026-10-04"}, Object{"kind": "date_range", "zone": "UTC", "startDate": "2026-10-05", "startAt": "bad"}, Object{"zone": "UTC", "startAt": "bad"}, Object{"zone": "UTC", "startAt": "2026-10-05T10:00:00Z", "endAt": "2026-10-05T10:00:00Z"}, Object{"zone": "UTC", "startAt": "2026-10-05T10:00:00Z", "offsetChoice": "invalid"}, Object{"zone": "UTC", "startAt": "2026-10-05T10:00:00Z", "localEnd": "2026-10-05T11:00:00"}, Object{"zone": "Europe/Berlin", "startAt": "2026-03-29T01:30:00Z", "localStart": "2026-03-29T02:30:00"}, Object{"zone": "UTC", "unknown": true}}
	for _, v := range bad {
		if validateSchedule(v) == nil {
			t.Fatalf("accepted %v", v)
		}
	}
}
func TestDeadlineValidationAndWarnings(t *testing.T) {
	for _, v := range []any{nil, Object{"kind": "date", "date": "2026-10-05", "zone": "UTC"}, Object{"kind": "instant", "at": "2026-10-05T10:00:00Z", "zone": "UTC"}} {
		if e := validateDeadline(v); e != nil {
			t.Fatal(e)
		}
	}
	for _, v := range []any{true, Object{}, Object{"kind": "bad", "zone": "UTC"}, Object{"kind": "date", "date": "bad", "zone": "UTC"}, Object{"kind": "instant", "at": "bad", "zone": "UTC"}, Object{"kind": "date", "date": "2026-10-05", "zone": "UTC", "unknown": 1}, Object{"kind": "instant", "at": "2026-10-05T10:00:00Z", "zone": "UTC", "unknown": 1}} {
		if validateDeadline(v) == nil {
			t.Fatal(v)
		}
	}
	for _, tc := range []struct {
		s, d Object
		want bool
	}{
		{nil, nil, false},
		{Object{"kind": "date_range", "startDate": "2026-03-29", "endDate": "2026-03-29", "zone": "Europe/Berlin"}, Object{"kind": "date", "date": "2026-03-29", "zone": "Europe/Berlin"}, false},
		{Object{"kind": "date_range", "startDate": "2026-03-30", "zone": "Europe/Berlin"}, Object{"kind": "date", "date": "2026-03-29", "zone": "Europe/Berlin"}, true},
		{Object{"startAt": "2026-10-05T10:00:00Z", "zone": "UTC"}, Object{"kind": "instant", "at": "2026-10-05T09:00:00Z", "zone": "UTC"}, true},
		{Object{"startAt": "2026-10-05T10:00:00Z", "endAt": "2026-10-05T12:00:00Z", "zone": "UTC"}, Object{"kind": "instant", "at": "2026-10-05T12:00:00Z", "zone": "UTC"}, false},
		{Object{"zone": "invalid"}, Object{"zone": "UTC"}, false},
	} {
		if got := deadlineExceeded(Object{"scheduled": tc.s, "deadline": tc.d}); got != tc.want {
			t.Fatalf("%v: %v", tc, got)
		}
	}
}
func TestSchedulingAndLinksAreAtomicAndIndependent(t *testing.T) {
	s := NewState()
	a := apply(t, &s, "task.create", Object{"title": "A"}).Result["taskId"].(string)
	b := apply(t, &s, "task.create", Object{"title": "B"}).Result["taskId"].(string)
	old := Copy(s.Tasks[a])
	plans := Copy(s.Plans)
	r := apply(t, &s, "task.save", Object{"taskId": a, "patch": Object{"scheduled": Object{"kind": "date_range", "startDate": "2026-10-05", "endDate": "2026-10-07", "zone": "Europe/Moscow"}, "deadline": Object{"kind": "date", "date": "2026-10-06", "zone": "Europe/Moscow"}, "relatedTaskIds": []any{b}}})
	if len(r.Warnings) != 1 || r.Warnings[0] != "SCHEDULE_EXCEEDS_DEADLINE" {
		t.Fatal(r)
	}
	if !reflect.DeepEqual(list(s.Tasks[b], "relatedTaskIds"), []any{a}) {
		t.Fatal(s.Tasks[b])
	}
	for _, key := range []string{"estimateMinutes", "remainingMinutes", "importance", "urgency", "reviewRequired", "lifecycleState", "completionEventId", "inboxAvailableAt"} {
		if !reflect.DeepEqual(old[key], s.Tasks[a][key]) {
			t.Fatal(key)
		}
	}
	if !reflect.DeepEqual(plans, Copy(s.Plans)) {
		t.Fatal("changed plans")
	}
	for _, links := range []any{nil, "bad", []any{a}, []any{b, b}, []any{1}} {
		reject(t, s, "task.save", Object{"taskId": a, "patch": Object{"relatedTaskIds": links}}, "VALIDATION_FAILED")
	}
	reject(t, s, "task.save", Object{"taskId": a, "patch": Object{"relatedTaskIds": []any{"missing"}}}, "ENTITY_NOT_FOUND")
	before := Copy(s)
	_, _, err := Apply(s, Command{OperationID: ID(), ExpectedRevision: s.Revision - 1, Command: "task.save", Payload: Object{"taskId": a, "patch": Object{"scheduled": nil}}}, time.Now())
	if err == nil || !reflect.DeepEqual(before, Copy(s)) {
		t.Fatal("conflict mutated")
	}
	apply(t, &s, "task.save", Object{"taskId": b, "patch": Object{"relatedTaskIds": []any{}}})
	if len(list(s.Tasks[a], "relatedTaskIds")) != 0 {
		t.Fatal("reverse link remained")
	}
	apply(t, &s, "task.save", Object{"taskId": a, "patch": Object{"scheduled": nil}})
	if s.Tasks[a]["deadline"] == nil {
		t.Fatal("cleared deadline with schedule")
	}
}
func TestLocalFoldChoice(t *testing.T) {
	for _, tc := range []struct {
		choice, at string
		valid      bool
	}{{"", "2026-10-25T00:30:00Z", false}, {"earlier", "2026-10-25T00:30:00Z", true}, {"later", "2026-10-25T01:30:00Z", true}, {"later", "2026-10-25T00:30:00Z", false}} {
		e := validateSchedule(Object{"startAt": tc.at, "localStart": "2026-10-25T02:30:00", "zone": "Europe/Berlin", "offsetChoice": tc.choice})
		if (e == nil) != tc.valid {
			t.Fatal(tc, e)
		}
	}
	loc, _ := time.LoadLocation("UTC")
	if validateLocalChoice(time.Now(), loc, "bad", "") == nil {
		t.Fatal("bad local accepted")
	}
}
