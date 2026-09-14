package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"spotter/internal/collectors/script"
	"spotter/internal/model"
)

type Collector struct {
	ScriptPath string
	Runner     script.Runner
}

func (c Collector) Name() string {
	return "calendar"
}

func (c Collector) Collect(ctx context.Context) (model.SourceData, error) {
	now := time.Now()
	start, end := eventRange(now)
	out, err := c.Runner.Run(ctx, c.ScriptPath, start.Format("02.01.2006 15:04:05"), end.Format("02.01.2006 15:04:05"))
	if err != nil {
		return model.SourceData{}, err
	}
	var events []model.CalendarEvent
	if err := json.Unmarshal(out, &events); err != nil {
		return model.SourceData{}, fmt.Errorf("parse calendar json: %w", err)
	}
	return model.SourceData{Calendar: events, CalendarFrom: start, CalendarTo: end}, nil
}

func eventRange(now time.Time) (time.Time, time.Time) {
	y, m, d := now.Date()
	todayStart := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	return todayStart, todayStart.AddDate(0, 0, 8)
}
