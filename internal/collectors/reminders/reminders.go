package reminders

import (
	"context"
	"encoding/json"
	"fmt"

	"spotter/internal/collectors/script"
	"spotter/internal/model"
)

type Collector struct {
	ScriptPath string
	Runner     script.Runner
}

func (c Collector) Name() string {
	return "reminders"
}

func (c Collector) Collect(ctx context.Context) (model.SourceData, error) {
	out, err := c.Runner.Run(ctx, c.ScriptPath)
	if err != nil {
		return model.SourceData{}, err
	}
	var reminders []model.Reminder
	if err := json.Unmarshal(out, &reminders); err != nil {
		return model.SourceData{}, fmt.Errorf("parse reminders json: %w", err)
	}
	return model.SourceData{Reminders: reminders}, nil
}
