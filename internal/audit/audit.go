package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"spotter/internal/model"
	"spotter/internal/planner"
)

type Store interface {
	SaveRefresh(ctx context.Context, record RefreshRecord) error
}

type JSONLStore struct {
	Path string
	mu   sync.Mutex
}

type RefreshRecord struct {
	Trigger     string          `json:"trigger,omitempty"`
	OperationID string          `json:"operationId"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Sources     []SourceRecord  `json:"sources"`
	Model       planner.Trace   `json:"model"`
	Plan        model.DailyPlan `json:"plan"`
}

type SourceRecord struct {
	Summary    model.SourceSummary `json:"summary"`
	Model      planner.Trace       `json:"model"`
	DurationMS int64               `json:"durationMs"`
	Name       string              `json:"name"`
	OK         bool                `json:"ok"`
	Error      string              `json:"error,omitempty"`
	UpdatedAt  time.Time           `json:"updatedAt"`
	Data       model.SourceData    `json:"data,omitempty"`
}

func (s *JSONLStore) SaveRefresh(ctx context.Context, record RefreshRecord) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if s == nil || s.Path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return fmt.Errorf("create audit dir: %w", err)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal audit record: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.OpenFile(s.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return nil
}
