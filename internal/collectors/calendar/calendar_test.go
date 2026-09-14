package calendar

import (
	"testing"
	"time"
)

func TestEventRangeCoversTodayAndNextWeek(t *testing.T) {
	now := time.Date(2026, 6, 17, 15, 30, 0, 0, time.FixedZone("MSK", 3*60*60))

	start, end := eventRange(now)

	wantStart := time.Date(2026, 6, 17, 0, 0, 0, 0, now.Location())
	wantEnd := time.Date(2026, 6, 25, 0, 0, 0, 0, now.Location())
	if !start.Equal(wantStart) {
		t.Fatalf("start = %s, want %s", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Fatalf("end = %s, want %s", end, wantEnd)
	}
}
