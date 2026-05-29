package scheduler

import (
	"context"
	"log/slog"
	"time"
)

type Scheduler struct {
	logger          *slog.Logger
	refreshInterval time.Duration
	planTime        string
	planEnabled     bool
	refresh         func(context.Context)
	generatePlan    func(context.Context)
}

func New(logger *slog.Logger, refreshInterval time.Duration, planEnabled bool, planTime string, refresh func(context.Context), generatePlan func(context.Context)) *Scheduler {
	return &Scheduler{
		logger:          logger,
		refreshInterval: refreshInterval,
		planEnabled:     planEnabled,
		planTime:        planTime,
		refresh:         refresh,
		generatePlan:    generatePlan,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	go s.refreshLoop(ctx)
	if s.planEnabled {
		go s.planLoop(ctx)
	}
}

func (s *Scheduler) refreshLoop(ctx context.Context) {
	if s.refreshInterval <= 0 {
		s.refreshInterval = 5 * time.Minute
	}
	ticker := time.NewTicker(s.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refresh(ctx)
		}
	}
}

func (s *Scheduler) planLoop(ctx context.Context) {
	for {
		next, err := nextPlanTime(time.Now(), s.planTime)
		if err != nil {
			s.logger.Warn("invalid daily plan time", "time", s.planTime, "error", err)
			return
		}
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.generatePlan(ctx)
		}
	}
}

func nextPlanTime(now time.Time, value string) (time.Time, error) {
	clock, err := time.Parse("15:04", value)
	if err != nil {
		return time.Time{}, err
	}
	next := time.Date(now.Year(), now.Month(), now.Day(), clock.Hour(), clock.Minute(), 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next, nil
}
