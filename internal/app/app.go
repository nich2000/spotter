package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"spotter/internal/collectors"
	"spotter/internal/model"
	"spotter/internal/planner"
	"spotter/internal/sse"
	"spotter/internal/storage"
)

type App struct {
	logger     *slog.Logger
	collectors []collectors.Collector
	planner    planner.Planner
	store      storage.Store
	broker     *sse.Broker

	mu    sync.RWMutex
	state model.AppState
}

func New(logger *slog.Logger, collectors []collectors.Collector, planner planner.Planner, store storage.Store, broker *sse.Broker) *App {
	return &App{
		logger:     logger,
		collectors: collectors,
		planner:    planner,
		store:      store,
		broker:     broker,
	}
}

func (a *App) Load(ctx context.Context) {
	state, err := a.store.Load(ctx)
	if err != nil {
		a.logger.Warn("load saved state failed", "error", err)
		return
	}
	if !state.GeneratedAt.IsZero() {
		a.setState(state)
		if err := a.broker.Publish(state); err != nil {
			a.logger.Warn("publish loaded state failed", "error", err)
		}
	}
}

func (a *App) State() model.AppState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

func (a *App) Refresh(ctx context.Context) model.AppState {
	a.logger.Info("refresh started")
	now := time.Now()
	next := model.AppState{
		GeneratedAt: now,
		Calendar:    []model.CalendarEvent{},
		Reminders:   []model.Reminder{},
		Mail:        []model.MailMessage{},
		Notes:       []model.Note{},
		Sources:     make([]model.SourceStatus, 0, len(a.collectors)),
	}

	for _, collector := range a.collectors {
		data, err := collector.Collect(ctx)
		status := model.SourceStatus{
			Name:      collector.Name(),
			OK:        err == nil,
			UpdatedAt: time.Now(),
		}
		if err != nil {
			status.Error = err.Error()
			a.logger.Warn("collector failed", "source", collector.Name(), "error", err)
		} else {
			next.Calendar = append(next.Calendar, data.Calendar...)
			next.Reminders = append(next.Reminders, data.Reminders...)
			next.Mail = append(next.Mail, data.Mail...)
			next.Notes = append(next.Notes, data.Notes...)
		}
		next.Sources = append(next.Sources, status)
	}

	plan, err := a.planner.Generate(ctx, next)
	if err != nil {
		a.logger.Warn("daily plan generation failed", "error", err)
	} else {
		next.Plan = plan
	}

	a.setState(next)
	if err := a.store.Save(ctx, next); err != nil {
		a.logger.Warn("save state failed", "error", err)
	}
	if err := a.broker.Publish(next); err != nil {
		a.logger.Warn("publish state failed", "error", err)
	}
	a.logger.Info("refresh completed", "sources", len(next.Sources))
	return next
}

func (a *App) GeneratePlan(ctx context.Context) {
	state := a.State()
	plan, err := a.planner.Generate(ctx, state)
	if err != nil {
		a.logger.Warn("scheduled plan generation failed", "error", err)
		return
	}
	state.GeneratedAt = time.Now()
	state.Plan = plan
	a.setState(state)
	if err := a.store.Save(ctx, state); err != nil {
		a.logger.Warn("save planned state failed", "error", err)
	}
	if err := a.broker.Publish(state); err != nil {
		a.logger.Warn("publish planned state failed", "error", err)
	}
}

func (a *App) setState(state model.AppState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = state
}
