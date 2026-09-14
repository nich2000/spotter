package app

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"spotter/internal/collectors"
	"spotter/internal/model"
	"spotter/internal/sse"
)

type memoryStore struct {
	state model.AppState
}

func (s *memoryStore) Load(ctx context.Context) (model.AppState, error) {
	return s.state, nil
}

func (s *memoryStore) Save(ctx context.Context, state model.AppState) error {
	s.state = state
	return nil
}

type staticPlanner struct {
	seen model.AppState
}

func (p *staticPlanner) Generate(ctx context.Context, input model.AppState) (model.DailyPlan, error) {
	p.seen = input
	return model.DailyPlan{Summary: "ok"}, nil
}

type blockingCollector struct {
	name       string
	started    chan<- string
	startCount *atomic.Int32
	release    <-chan struct{}
	data       model.SourceData
}

func (c blockingCollector) Name() string {
	return c.name
}

func (c blockingCollector) Collect(ctx context.Context) (model.SourceData, error) {
	c.startCount.Add(1)
	c.started <- c.name
	select {
	case <-ctx.Done():
		return model.SourceData{}, ctx.Err()
	case <-c.release:
		return c.data, nil
	}
}

func TestRefreshCollectsSourcesInParallelBeforePlanning(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	var startCount atomic.Int32

	items := []collectors.Collector{
		blockingCollector{name: "calendar", started: started, startCount: &startCount, release: release, data: model.SourceData{Calendar: []model.CalendarEvent{{Title: "event"}}}},
		blockingCollector{name: "reminders", started: started, startCount: &startCount, release: release, data: model.SourceData{Reminders: []model.Reminder{{Title: "task"}}}},
		blockingCollector{name: "mail", started: started, startCount: &startCount, release: release, data: model.SourceData{Mail: []model.MailMessage{{Subject: "mail", IsUnread: true}}}},
		blockingCollector{name: "notes", started: started, startCount: &startCount, release: release, data: model.SourceData{Notes: []model.Note{{Title: "note"}}}},
	}
	plan := &staticPlanner{}
	app := New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		items,
		plan,
		&memoryStore{},
		nil,
		sse.NewBroker(slog.New(slog.NewTextHandler(io.Discard, nil))),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var once sync.Once
	go func() {
		for i := 0; i < len(items); i++ {
			select {
			case <-started:
			case <-ctx.Done():
				return
			}
		}
		once.Do(func() { close(release) })
	}()

	state := app.Refresh(ctx)

	if got := startCount.Load(); got != int32(len(items)) {
		t.Fatalf("started collectors = %d, want %d", got, len(items))
	}
	if len(state.Sources) != len(items) {
		t.Fatalf("sources = %d, want %d", len(state.Sources), len(items))
	}
	if len(plan.seen.Calendar) != 1 || len(plan.seen.Reminders) != 1 || len(plan.seen.Mail) != 1 || len(plan.seen.Notes) != 1 {
		t.Fatalf("planner input did not include all collector data: %+v", plan.seen)
	}
	for i, source := range state.Sources {
		if source.Name != items[i].Name() {
			t.Fatalf("source[%d] = %s, want %s", i, source.Name, items[i].Name())
		}
	}
}
