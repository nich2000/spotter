package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"spotter/internal/audit"
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
	auditStore audit.Store
	broker     *sse.Broker

	operationMu sync.Mutex
	mu          sync.RWMutex
	state       model.AppState
}

type collectResult struct {
	summary      model.SourceSummary
	summaryTrace planner.Trace
	name         string
	data         model.SourceData
	err          error
	updatedAt    time.Time
	durationMS   int64
}

func New(logger *slog.Logger, collectors []collectors.Collector, planner planner.Planner, store storage.Store, auditStore audit.Store, broker *sse.Broker) *App {
	return &App{
		logger:     logger,
		collectors: collectors,
		planner:    planner,
		store:      store,
		auditStore: auditStore,
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
	ctx, id := planner.WithOperation(ctx)
	logger := a.logger.With("operation_id", id, "trigger", planner.Trigger(ctx))
	queuedAt := time.Now()
	logger.Info("operation queued")
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	logger.Info("operation acquired", "queue_ms", time.Since(queuedAt).Milliseconds())
	if ctx.Err() != nil {
		logger.Warn("refresh canceled before start", "error", ctx.Err())
		return a.State()
	}
	logger.Info("refresh started", "sources", len(a.collectors))
	failures := 0
	now := time.Now()
	summarizer, summarize := a.planner.(planner.SourceSummarizer)
	next := model.AppState{
		GeneratedAt: now,
		Calendar:    []model.CalendarEvent{},
		Reminders:   []model.Reminder{},
		Mail:        []model.MailMessage{},
		Notes:       []model.Note{},
		Sources:     make([]model.SourceStatus, 0, len(a.collectors)),
	}
	if summarize {
		next.SourceSummaries = make([]model.SourceSummary, 0, len(a.collectors))
	}
	sourceRecords := make([]audit.SourceRecord, 0, len(a.collectors))
	results := make([]collectResult, len(a.collectors))

	slots := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, collector := range a.collectors {
		wg.Add(1)
		go func(i int, collector collectors.Collector) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()

			started := time.Now()
			logger.Info("collector started", "source", collector.Name())
			data, err := collector.Collect(ctx)
			attrs := []any{"source", collector.Name(), "duration_ms", time.Since(started).Milliseconds(), "calendar", len(data.Calendar), "reminders", len(data.Reminders), "mail", len(data.Mail), "notes", len(data.Notes)}
			if err != nil {
				logger.Warn("collector failed", append(attrs, "error", err)...)
			} else {
				logger.Info("collector completed", attrs...)
			}
			results[i] = collectResult{
				name:       collector.Name(),
				data:       data,
				err:        err,
				updatedAt:  time.Now(),
				durationMS: time.Since(started).Milliseconds(),
			}
			if summarize {
				if err != nil {
					results[i].summary = model.SourceSummary{Name: collector.Name(), Status: "skipped", Error: err.Error(), GeneratedAt: time.Now()}
					logger.Warn("source summary skipped", "source", collector.Name(), "reason", "collection_failed")
				} else {
					logger.Info("source summary started", "source", collector.Name())
					results[i].summary, results[i].summaryTrace = summarizer.SummarizeSource(ctx, collector.Name(), data, results[i].updatedAt)
					logger.Info("source summary completed", "source", collector.Name(), "status", results[i].summary.Status, "error", results[i].summary.Error)
				}
			}

		}(i, collector)
	}
	wg.Wait()

	for _, result := range results {
		status := model.SourceStatus{
			Name:      result.name,
			OK:        result.err == nil,
			UpdatedAt: result.updatedAt,
		}
		if result.err != nil {
			status.Error = result.err.Error()
			failures++
		} else {
			next.Calendar = append(next.Calendar, result.data.Calendar...)
			if result.name == "calendar" {
				next.CalendarFrom = result.data.CalendarFrom
				next.CalendarTo = result.data.CalendarTo
			}
			next.Reminders = append(next.Reminders, result.data.Reminders...)
			next.Mail = append(next.Mail, result.data.Mail...)
			next.Notes = append(next.Notes, result.data.Notes...)
		}
		next.Sources = append(next.Sources, status)
		if summarize {
			next.SourceSummaries = append(next.SourceSummaries, result.summary)
			if result.summary.Status == "failed" {
				failures++
			}
		}
		sourceRecords = append(sourceRecords, audit.SourceRecord{
			Summary:    result.summary,
			Model:      result.summaryTrace,
			DurationMS: result.durationMS,
			Name:       status.Name,
			OK:         status.OK,
			Error:      status.Error,
			UpdatedAt:  status.UpdatedAt,
			Data:       result.data,
		})
	}

	plan, trace, err := planner.GenerateWithTrace(ctx, a.planner, next)
	if err != nil {
		failures++
		trace.Error = err.Error()
		logger.Warn("daily plan generation failed", "error", err)
	} else {
		next.Plan = plan
	}
	if err == nil && trace.Error != "" {
		logger.Warn("daily plan unavailable", "stage", trace.Stage, "error", trace.Error)
		failures++
	}
	logger.Info("plan result", "status", trace.Status, "backend", trace.Backend, "blocks", len(next.Plan.Blocks), "risks", len(next.Plan.Risks), "focus", len(next.Plan.Focus))
	if a.auditStore != nil {
		record := audit.RefreshRecord{
			OperationID: id,
			Trigger:     planner.Trigger(ctx),
			GeneratedAt: next.GeneratedAt,
			Sources:     sourceRecords,
			Model:       trace,
			Plan:        next.Plan,
		}
		if err := a.auditStore.SaveRefresh(ctx, record); err != nil {
			failures++
			logger.Warn("save refresh audit failed", "error", err)
		} else {
			logger.Info("audit saved")
		}
	}

	a.setState(next)
	if err := a.store.Save(ctx, next); err != nil {
		failures++
		logger.Warn("save state failed", "error", err)
	} else {
		logger.Info("state saved")
	}
	if err := a.broker.Publish(next); err != nil {
		failures++
		logger.Warn("publish state failed", "error", err)
	} else {
		logger.Info("state published")
	}
	status := "ok"
	if failures > 0 {
		status = "degraded"
	}
	level := slog.LevelInfo
	if failures > 0 {
		level = slog.LevelWarn
	}
	logger.Log(ctx, level, "refresh completed", "status", status, "failures", failures, "duration_ms", time.Since(now).Milliseconds(), "sources", len(next.Sources))
	return next
}

func (a *App) GeneratePlan(ctx context.Context) {
	// Legacy saved states have no summaries: collect and summarize before synthesis.
	if _, ok := a.planner.(planner.SourceSummarizer); ok && a.State().SourceSummaries == nil {
		a.Refresh(ctx)
		return
	}

	ctx, id := planner.WithOperation(ctx)
	logger := a.logger.With("operation_id", id, "trigger", planner.Trigger(ctx))
	queuedAt := time.Now()
	logger.Info("operation queued")
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	logger.Info("operation acquired", "queue_ms", time.Since(queuedAt).Milliseconds())
	if ctx.Err() != nil {
		logger.Warn("scheduled plan canceled before start", "error", ctx.Err())
		return
	}
	started := time.Now()
	failures := 0
	logger.Info("scheduled plan started")
	state := a.State()
	plan, trace, err := planner.GenerateWithTrace(ctx, a.planner, state)
	if err != nil {
		logger.Warn("scheduled plan generation failed", "error", err)
		return
	}
	state.GeneratedAt = time.Now()
	state.Plan = plan
	if a.auditStore != nil {
		if err := a.auditStore.SaveRefresh(ctx, audit.RefreshRecord{OperationID: id,
			Trigger: planner.Trigger(ctx), GeneratedAt: state.GeneratedAt, Model: trace, Plan: plan}); err != nil {
			failures++
			logger.Warn("save scheduled plan audit failed", "error", err)
		}
	}
	if trace.Error != "" {
		failures++
		logger.Warn("scheduled plan unavailable", "model_status", trace.Status, "error", trace.Error)
	}
	a.setState(state)
	if err := a.store.Save(ctx, state); err != nil {
		failures++
		logger.Warn("save planned state failed", "error", err)
	}
	if err := a.broker.Publish(state); err != nil {
		failures++
		logger.Warn("publish planned state failed", "error", err)
	}
	status, level := "ok", slog.LevelInfo
	if failures > 0 {
		status, level = "degraded", slog.LevelWarn
	}
	logger.Log(ctx, level, "scheduled plan completed", "status", status, "failures", failures, "duration_ms", time.Since(started).Milliseconds())
}

func (a *App) setState(state model.AppState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = state
}
