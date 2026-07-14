// Package collector defines cost collection plugins and their registry.
package collector

import (
	"context"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

// Collector refreshes one bounded family of AWS cost data.
type Collector interface {
	Name() string
	Collect(ctx context.Context, reference time.Time) (cost.PartialSnapshot, error)
}

// Factory constructs a collector with dependencies captured by its closure.
type Factory func() (Collector, error)
