package scheduler

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestSchedule(t *testing.T) {
	now := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	next, e := nextPlanTime(now, "11:00")
	if e != nil || next.Sub(now) != time.Hour {
		t.Fatal(next, e)
	}
	next, e = nextPlanTime(now, "09:00")
	if e != nil || next.Sub(now) != 23*time.Hour {
		t.Fatal(next, e)
	}
	if _, e = nextPlanTime(now, "bad"); e == nil {
		t.Fatal("bad clock")
	}
	ctx, cancel := context.WithCancel(context.Background())
	hit := make(chan struct{}, 1)
	s := New(slog.New(slog.NewTextHandler(io.Discard, nil)), time.Millisecond, true, "bad", func(context.Context) {
		select {
		case hit <- struct{}{}:
		default:
		}
	}, func(context.Context) {})
	s.Start(ctx)
	select {
	case <-hit:
	case <-time.After(time.Second):
		t.Fatal("refresh not called")
	}
	cancel()
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	stopped := New(slog.New(slog.NewTextHandler(io.Discard, nil)), 0, true, "12:00", func(context.Context) {}, func(context.Context) {})
	stopped.refreshLoop(ctx2)
	stopped.planLoop(ctx2)
}
