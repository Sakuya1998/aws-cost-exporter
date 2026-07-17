// Package total collects ungrouped daily and month-to-date costs.
package total

import (
	"context"
	"errors"
	"fmt"
	"time"

	basecollector "github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

const (
	// Name is the stable registry and telemetry identifier.
	Name = "total"
)

// ErrNilReader indicates a missing Cost Explorer dependency.
var ErrNilReader = errors.New("total cost reader must not be nil")

// Reader is the narrow cost-reading port required by this collector.
type Reader interface {
	ReadCosts(context.Context, ports.CostQuery) ([]cost.Cost, error)
}

// Collector retrieves ungrouped total costs.
type Collector struct {
	id     identity.CollectorID
	reader Reader
}

// New validates dependencies and constructs a total collector.
func New(reader Reader) (*Collector, error) {
	return NewForTarget("default", reader)
}

// NewForTarget constructs a target-scoped total collector.
func NewForTarget(target identity.TargetID, reader Reader) (*Collector, error) {
	if reader == nil {
		return nil, ErrNilReader
	}
	id := identity.CollectorID{Target: target, Name: Name}
	if !id.Valid() {
		return nil, errors.New("invalid total collector target")
	}
	return &Collector{id: id, reader: reader}, nil
}

// Name returns the stable collector identifier.
func (collector *Collector) Name() string {
	return Name
}

// ID returns the target-scoped collector identity.
func (collector *Collector) ID() identity.CollectorID { return collector.id }

// Collect retrieves daily and month-to-date totals as one publishable result.
func (collector *Collector) Collect(
	ctx context.Context,
	reference time.Time,
) (snapshot.PartialSnapshot, error) {
	queries, err := basecollector.BuildDailyAndMTDQueries(reference, cost.DimensionTotal)
	if err != nil {
		return snapshot.PartialSnapshot{}, err
	}

	var collected []cost.Cost
	for _, query := range queries {
		if err := ctx.Err(); err != nil {
			return snapshot.PartialSnapshot{}, err
		}
		values, err := collector.reader.ReadCosts(ctx, query)
		if err != nil {
			return snapshot.PartialSnapshot{}, fmt.Errorf("collect %s total cost: %w", query.Window, err)
		}
		for index := range values {
			values[index].Target = collector.id.Target
		}
		collected = append(collected, values...)
	}
	return snapshot.New(collected, nil, nil, nil), nil
}
