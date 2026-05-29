package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"spotter/internal/model"
)

type Store interface {
	Load(ctx context.Context) (model.AppState, error)
	Save(ctx context.Context, state model.AppState) error
}

type JSONStore struct {
	Path string
}

func (s JSONStore) Load(ctx context.Context) (model.AppState, error) {
	select {
	case <-ctx.Done():
		return model.AppState{}, ctx.Err()
	default:
	}

	raw, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.AppState{}, nil
		}
		return model.AppState{}, fmt.Errorf("read state: %w", err)
	}

	var state model.AppState
	if err := json.Unmarshal(raw, &state); err != nil {
		return model.AppState{}, fmt.Errorf("parse state: %w", err)
	}
	return state, nil
}

func (s JSONStore) Save(ctx context.Context, state model.AppState) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(s.Path, raw, 0o600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}
