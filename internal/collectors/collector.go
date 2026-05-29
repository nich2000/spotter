package collectors

import (
	"context"

	"spotter/internal/model"
)

type Collector interface {
	Name() string
	Collect(ctx context.Context) (model.SourceData, error)
}
