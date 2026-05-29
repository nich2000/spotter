package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"spotter/internal/collectors/script"
	"spotter/internal/model"
)

type Collector struct {
	ScriptPath string
	Limit      int
	Runner     script.Runner
}

func (c Collector) Name() string {
	return "mail"
}

func (c Collector) Collect(ctx context.Context) (model.SourceData, error) {
	limit := c.Limit
	if limit <= 0 {
		limit = 20
	}
	out, err := c.Runner.Run(ctx, c.ScriptPath, strconv.Itoa(limit))
	if err != nil {
		return model.SourceData{}, err
	}
	var messages []model.MailMessage
	if err := json.Unmarshal(out, &messages); err != nil {
		return model.SourceData{}, fmt.Errorf("parse mail json: %w", err)
	}
	unread := make([]model.MailMessage, 0, len(messages))
	for _, message := range messages {
		if message.IsUnread {
			unread = append(unread, message)
		}
	}
	return model.SourceData{Mail: unread}, nil
}
