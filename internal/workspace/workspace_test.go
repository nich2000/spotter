package workspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"spotter/internal/model"
)

func testNow() time.Time { return time.Date(2026, 9, 14, 6, 0, 0, 0, time.UTC) }
func newService(t *testing.T) *Service {
	t.Helper()
	s, e := New(filepath.Join(t.TempDir(), "work.json"))
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func apply(t *testing.T, s *Service, c Command) {
	t.Helper()
	c.Version = s.Snapshot().Version
	if e := s.Apply(c, model.AppState{}, testNow()); e != nil {
		t.Fatal(e)
	}
}
func add(t *testing.T, s *Service, title string) Task {
	t.Helper()
	apply(t, s, Command{Action: "capture", Task: Task{Title: title, Minutes: 90}})
	d := s.Snapshot()
	return d.Tasks[len(d.Tasks)-1]
}
func calendarState(now time.Time) model.AppState {
	return model.AppState{GeneratedAt: now, CalendarFrom: now.Add(-6 * time.Hour), CalendarTo: now.AddDate(0, 0, 8), Sources: []model.SourceStatus{{Name: "calendar", OK: true, UpdatedAt: now}}}
}
func TestLimitsAndPersistence(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		count        int
		mit, weekly  bool
	}{{"doing", "doing", 3, false, false}, {"ready", "ready", 15, false, false}, {"mit", "ready", 3, true, false}, {"week", "backlog", 5, false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			s := newService(t)
			for i := 0; i <= tc.count; i++ {
				v := add(t, s, "Действие")
				v.Project = "Work"
				v.Status = tc.status
				v.Weekly = tc.weekly
				if tc.mit {
					v.MITDate = "2026-09-14"
				}
				before := s.Snapshot()
				e := s.Apply(Command{Version: before.Version, Action: "update", Task: v}, model.AppState{}, testNow())
				if i < tc.count && e != nil {
					t.Fatal(e)
				}
				if i == tc.count {
					if e == nil {
						t.Fatal("limit accepted")
					}
					if s.Snapshot().Version != before.Version {
						t.Fatal("failed mutation changed version")
					}
				}
			}
			loaded, e := New(s.path)
			if e != nil {
				t.Fatal(e)
			}
			if loaded.Snapshot().Version != s.Snapshot().Version {
				t.Fatal("not persisted")
			}
		})
	}
}
func TestConcurrentVersionAndSourceDedup(t *testing.T) {
	s := newService(t)
	state := model.AppState{Reminders: []model.Reminder{{Title: "A", List: "Work"}}, Mail: []model.MailMessage{{ID: 1, Subject: "B"}}, Notes: []model.Note{{Title: "C"}}, Calendar: []model.CalendarEvent{{Title: "D", Start: testNow()}}}
	items := Candidates(state, s.Snapshot())
	if len(items) != 4 {
		t.Fatal(items)
	}
	c := Command{Version: 0, Action: "import", Key: items[0].Key}
	if e := s.Apply(c, state, testNow()); e != nil {
		t.Fatal(e)
	}
	if e := s.Apply(c, state, testNow()); e != ErrConflict {
		t.Fatal(e)
	}
	if len(Candidates(state, s.Snapshot())) != 3 {
		t.Fatal("duplicate candidate")
	}
	d := s.Snapshot()
	d.Tasks[0].Title = "outside mutation"
	if s.Snapshot().Tasks[0].Title == d.Tasks[0].Title {
		t.Fatal("snapshot aliases state")
	}
}
func TestWriteFailureDoesNotMutate(t *testing.T) {
	s := newService(t)
	parent := filepath.Join(t.TempDir(), "file")
	if e := os.WriteFile(parent, []byte("x"), 0600); e != nil {
		t.Fatal(e)
	}
	s.path = filepath.Join(parent, "work")
	if e := s.Apply(Command{Action: "capture", Task: Task{Title: "test", Minutes: 15}}, model.AppState{}, testNow()); e == nil {
		t.Fatal("expected write failure")
	}
	if len(s.Snapshot().Tasks) != 0 {
		t.Fatal("mutation despite failed save")
	}
}
func TestScheduleConflictsBudgetAndCoverage(t *testing.T) {
	s := newService(t)
	now := testNow()
	d := s.Snapshot()
	d.Tasks = []Task{{ID: "a", Title: "Firmware", Status: "ready", Minutes: 300, MITDate: "2026-09-14"}, {ID: "b", Title: "Backend", Status: "ready", Minutes: 180, MITDate: "2026-09-14"}}
	state := calendarState(now)
	// Overlapping meetings must count once when computing available time.
	state.Calendar = []model.CalendarEvent{{Title: "A", Start: now.Add(4 * time.Hour), End: now.Add(5 * time.Hour)}, {Title: "B", Start: now.Add(4*time.Hour + 30*time.Minute), End: now.Add(6 * time.Hour)}}
	p := Build(d, state, "2026-09-14", now)
	if p.Free != 390 || p.Budget != 292 {
		t.Fatalf("free/budget: %+v", p)
	}
	used := 0
	for _, b := range p.Blocks {
		if b.Kind == "calendar" {
			continue
		}
		used += int(b.End.Sub(b.Start).Minutes())
		for _, e := range state.Calendar {
			if b.Start.Before(e.End) && b.End.After(e.Start) {
				t.Fatal("overlap")
			}
		}
	}
	if used > p.Budget || len(p.Warnings) == 0 {
		t.Fatalf("budget/unscheduled: %+v", p)
	}
	state.Sources[0].OK = false
	if len(Build(d, state, "2026-09-14", now).Blocks) != 0 {
		t.Fatal("scheduled with failed source")
	}
	state.Sources[0].OK = true
	if len(Build(d, state, "2026-09-23", now).Blocks) != 0 {
		t.Fatal("scheduled beyond coverage")
	}
	state.Sources[0].UpdatedAt = now.Add(-25 * time.Hour)
	if len(Build(d, state, "2026-09-14", now).Blocks) != 0 {
		t.Fatal("stale calendar")
	}
}
func TestEnergyIsDatedAndMissingIsUnknown(t *testing.T) {
	s := newService(t)
	now := testNow()
	d := s.Snapshot()
	hours := 6.
	d.Health = &Health{SleepHours: &hours, SleepEnd: now.Add(-time.Hour)}
	d.Tasks = []Task{{ID: "a", Title: "A", Status: "ready", Minutes: 90, MITDate: "2026-09-14"}}
	p := Build(d, calendarState(now), "2026-09-14", now)
	if p.Mode != "gentle" {
		t.Fatal(p)
	}
	for _, b := range p.Blocks {
		if b.Kind == "focus" && b.End.Sub(b.Start) > 45*time.Minute {
			t.Fatal("long block")
		}
	}
	d.Energy = "normal"
	d.EnergyDate = "2026-09-14"
	if Build(d, calendarState(now), "2026-09-14", now).Mode != "normal" {
		t.Fatal("manual override ignored")
	}
	d.EnergyDate = "2026-09-13"
	d.Health.SleepEnd = now.Add(-48 * time.Hour)
	if Build(d, calendarState(now), "2026-09-14", now).Mode != "normal" {
		t.Fatal("stale input used")
	}
}
func TestHealthUnionEmptyAndInvalid(t *testing.T) {
	now := testNow()
	in := HealthInput{MeasuredAt: now, Sleep: []SleepSample{{now.Add(-8 * time.Hour), now.Add(-time.Hour)}, {now.Add(-7 * time.Hour), now.Add(-2 * time.Hour)}}}
	raw, _ := json.Marshal(in)
	h, e := ParseHealth(bytes.NewReader(raw), now)
	if e != nil || h.SleepHours == nil || *h.SleepHours != 7 {
		t.Fatalf("dedup: %+v %v", h, e)
	}
	in.Sleep = nil
	raw, _ = json.Marshal(in)
	h, e = ParseHealth(bytes.NewReader(raw), now)
	if e != nil || h.SleepHours != nil {
		t.Fatal("missing treated as zero")
	}
	for _, raw := range []string{`{}`, `{"measuredAt":"2026-09-14T06:00:00Z","sleep":[],"secret":1}`, `{"measuredAt":"2027-01-01T00:00:00Z","sleep":[]}`, `{"measuredAt":"2026-09-14T06:00:00Z","sleep":[]} {}`} {
		if _, e = ParseHealth(strings.NewReader(raw), now); e == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
func TestBridgePreservesLatestAndRejectsPartialWrite(t *testing.T) {
	s := newService(t)
	path := filepath.Join(t.TempDir(), "health.json")
	b := &Bridge{Path: path, Service: s}
	now := testNow()
	in := HealthInput{MeasuredAt: now, Sleep: []SleepSample{{now.Add(-8 * time.Hour), now.Add(-time.Hour)}}}
	raw, _ := json.Marshal(in)
	os.WriteFile(path, raw, 0600)
	b.Sync(now)
	if s.Snapshot().Health == nil {
		t.Fatal(b.Message())
	}
	v := s.Snapshot().Version
	b.Sync(now)
	if s.Snapshot().Version != v {
		t.Fatal("duplicate import")
	}
	os.WriteFile(path, []byte("{"), 0600)
	b.Sync(now)
	if s.Snapshot().Version != v {
		t.Fatal("bad import overwrote state")
	}
	in.MeasuredAt = now.Add(-time.Hour)
	in.Sleep = nil
	raw, _ = json.Marshal(in)
	os.WriteFile(path, raw, 0600)
	b.Sync(now)
	if s.Snapshot().Version != v {
		t.Fatal("older import overwrote state")
	}
}

func TestCompletedMITStillCountsForDay(t *testing.T) {
	s := newService(t)
	for i := 0; i < 3; i++ {
		task := add(t, s, "Result")
		task.Project = "Work"
		task.Status = "ready"
		task.MITDate = "2026-09-14"
		apply(t, s, Command{Action: "update", Task: task})
		task.Status = "done"
		apply(t, s, Command{Action: "update", Task: task})
	}
	if plan := Build(s.Snapshot(), calendarState(testNow()), "2026-09-14", testNow()); plan.Planned != 0 {
		t.Fatal("completed results were scheduled again")
	}
	task := add(t, s, "Fourth")
	task.Project = "Work"
	task.Status = "ready"
	task.MITDate = "2026-09-14"
	if err := s.Apply(Command{Version: s.Snapshot().Version, Action: "update", Task: task}, model.AppState{}, testNow()); err == nil {
		t.Fatal("completed MITs allowed a fourth daily result")
	}
}

func TestMainResultSelectionAndScheduleOrder(t *testing.T) {
	s := newService(t)
	a, b := add(t, s, "Extra"), add(t, s, "Main")
	for _, task := range []*Task{&a, &b} {
		task.Status = "ready"
		task.MITDate = "2026-09-14"
		apply(t, s, Command{Action: "update", Task: *task})
	}
	apply(t, s, Command{Action: "main", Task: Task{ID: b.ID}, Date: "2026-09-14"})
	p := Build(s.Snapshot(), calendarState(testNow()), "2026-09-14", testNow())
	if len(p.Blocks) == 0 || p.Blocks[0].TaskID != b.ID {
		t.Fatalf("main not scheduled first: %+v", p)
	}
	apply(t, s, Command{Action: "main", Task: Task{ID: a.ID}, Date: "2026-09-14"})
	d := s.Snapshot()
	if d.Tasks[0].MainDate != "2026-09-14" || d.Tasks[1].MainDate != "" || d.Tasks[1].MITDate != "2026-09-14" {
		t.Fatal("replacement lost additional task", d.Tasks)
	}
	v := d.Tasks[0]
	v.Status = "backlog"
	v.Resume = "log URL; next experiment"
	apply(t, s, Command{Action: "update", Task: v})
	loaded, err := New(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Snapshot().Tasks[0].MainDate != "" || loaded.Snapshot().Tasks[0].Resume != v.Resume {
		t.Fatal("resume or main persisted incorrectly")
	}
}

func TestWaitingCountsAsWIPAndNeedsFollowup(t *testing.T) {
	s := newService(t)
	for i := 0; i < 3; i++ {
		v := add(t, s, "Experiment")
		v.Status = "doing"
		apply(t, s, Command{Action: "update", Task: v})
		v.Status = "waiting"
		if err := s.Apply(Command{Version: s.Snapshot().Version, Action: "update", Task: v}, model.AppState{}, testNow()); err == nil {
			t.Fatal("waiting accepted without followup")
		}
		v.WaitingOn = "Measurement"
		v.CheckDate = "2026-09-15"
		apply(t, s, Command{Action: "update", Task: v})
	}
	v := add(t, s, "Fourth")
	v.Status = "doing"
	if err := s.Apply(Command{Version: s.Snapshot().Version, Action: "update", Task: v}, model.AppState{}, testNow()); err == nil {
		t.Fatal("waiting bypassed WIP")
	}
}

func TestCandidateGroupingDismissRestoreAndLegacyImport(t *testing.T) {
	s := newService(t)
	state := model.AppState{Mail: []model.MailMessage{
		{ID: 11, Sender: "ci", Subject: "repo | Failed pipeline for dev | abc1234"},
		{ID: 12, Sender: "ci", Subject: "repo | Failed pipeline for v1 | abc1234"},
		{ID: 13, Sender: "ci", Subject: "other | Failed pipeline for dev | abc1234"},
	}}
	items := Candidates(state, s.Snapshot())
	if len(items) != 2 || items[0].Count != 2 || !strings.HasPrefix(items[0].Title, "Проверить сбой") {
		t.Fatal(items)
	}
	if err := s.Apply(Command{Action: "dismiss", Key: items[0].Key}, state, testNow()); err != nil {
		t.Fatal(err)
	}
	loaded, err := New(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if len(Candidates(state, loaded.Snapshot())) != 1 {
		t.Fatal("dismiss not persisted")
	}
	apply(t, s, Command{Action: "restoreCandidates"})
	if len(Candidates(state, s.Snapshot())) != 2 {
		t.Fatal("restore failed")
	}
	sum := sha256.Sum256([]byte("mail\x0011"))
	d := s.Snapshot()
	d.Tasks = []Task{{SourceKey: fmt.Sprintf("%x", sum[:16])}}
	if len(Candidates(state, d)) != 1 {
		t.Fatal("old imported message resurfaced as group")
	}
}

func TestUnicodeLimitsAndWaitingMetadataEdits(t *testing.T) {
	s := newService(t)
	v := add(t, s, strings.Repeat("я", 500))
	v.Status = "waiting"
	v.WaitingOn = strings.Repeat("ж", 500)
	v.CheckDate = "2026-09-15"
	v.Resume = strings.Repeat("я", 4000)
	apply(t, s, Command{Action: "update", Task: v})
	for _, field := range []string{"title", "resume", "waitingOn", "checkDate"} {
		bad := v
		switch field {
		case "title":
			bad.Title += "я"
		case "resume":
			bad.Resume += "я"
		case "waitingOn":
			bad.WaitingOn = ""
		case "checkDate":
			bad.CheckDate = ""
		}
		before := s.Snapshot().Version
		if err := s.Apply(Command{Version: before, Action: "update", Task: bad}, model.AppState{}, testNow()); err == nil {
			t.Fatalf("invalid %s accepted", field)
		}
		if s.Snapshot().Version != before {
			t.Fatal("rejected edit changed version")
		}
	}
}
