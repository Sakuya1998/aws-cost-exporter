// Package collector defines cost collection plugins and their registry.
package collector

import (
	"context"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
)

// Collector refreshes one bounded family of AWS cost data.
type Collector interface {
	ID() identity.CollectorID
	Collect(ctx context.Context, reference time.Time) (snapshot.PartialSnapshot, error)
}

// Factory constructs a collector with dependencies captured by its closure.
type Factory func() (Collector, error)
