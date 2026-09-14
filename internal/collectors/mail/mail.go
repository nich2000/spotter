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
	unread, err := parseMailMessages(out)
	if err != nil {
		return model.SourceData{}, err
	}
	return model.SourceData{Mail: unread}, nil
}

func parseMailMessages(out []byte) ([]model.MailMessage, error) {
	var messages []model.MailMessage
	if err := json.Unmarshal(out, &messages); err != nil {
		return nil, fmt.Errorf("parse mail json: %w", err)
	}
	seen := make(map[int]struct{}, len(messages))
	unread := make([]model.MailMessage, 0, len(messages))
	for _, message := range messages {
		if !message.IsUnread {
			continue
		}
		if message.ID != 0 {
			if _, ok := seen[message.ID]; ok {
				continue
			}
			seen[message.ID] = struct{}{}
		}
		unread = append(unread, message)
	}
	return unread, nil
}
