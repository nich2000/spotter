package notes

import (
	"context"
	"encoding/json"
	"fmt"

	"spotter/internal/collectors/script"
	"spotter/internal/model"
)

type Collector struct {
	ScriptPath string
	Folder     string
	Runner     script.Runner
}

func (c Collector) Name() string {
	return "notes"
}

func (c Collector) Collect(ctx context.Context) (model.SourceData, error) {
	if c.Folder == "" {
		return model.SourceData{}, fmt.Errorf("notes folder is empty")
	}
	out, err := c.Runner.Run(ctx, c.ScriptPath, c.Folder)
	if err != nil {
		return model.SourceData{}, err
	}
	var notes []model.Note
	if err := json.Unmarshal(out, &notes); err != nil {
		return model.SourceData{}, fmt.Errorf("parse notes json: %w", err)
	}
	return model.SourceData{Notes: notes}, nil
}
