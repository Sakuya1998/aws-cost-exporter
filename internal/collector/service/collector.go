// Package service collects service-grouped daily and month-to-date costs.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	basecollector "github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

const (
	// Name is the stable registry and telemetry identifier.
	Name = "service"
	// OtherService is the bounded label for aggregated overflow.
	OtherService = "__other__"
)

var (
	// ErrNilReader indicates a missing Cost Explorer dependency.
	ErrNilReader = errors.New("service cost reader must not be nil")
	// ErrInvalidSeriesLimit preserves the collector-specific public contract.
	ErrInvalidSeriesLimit = basecollector.ErrInvalidSeriesLimit
	// ErrMixedCurrency preserves the collector-specific public contract.
	ErrMixedCurrency = basecollector.ErrMixedCurrency
)

// Reader is the narrow cost-reading port required by this collector.
type Reader interface {
	ReadCosts(context.Context, ports.CostQuery) ([]cost.Cost, error)
}

// Collector retrieves service-grouped costs under a bounded series budget.
type Collector struct {
	reader      Reader
	seriesLimit int
	observers   []basecollector.OverflowObserver
}

// New validates dependencies and constructs a service collector.
func New(reader Reader, seriesLimit int, observers ...basecollector.OverflowObserver) (*Collector, error) {
	if reader == nil {
		return nil, ErrNilReader
	}
	if seriesLimit <= 0 {
		return nil, ErrInvalidSeriesLimit
	}
	return &Collector{reader: reader, seriesLimit: seriesLimit, observers: append([]basecollector.OverflowObserver(nil), observers...)}, nil
}

// Name returns the stable collector identifier.
func (collector *Collector) Name() string { return Name }

// Collect retrieves and cardinality-limits daily and month-to-date costs.
func (collector *Collector) Collect(
	ctx context.Context,
	reference time.Time,
) (cost.PartialSnapshot, error) {
	queries, err := basecollector.BuildDailyAndMTDQueries(reference, cost.DimensionService)
	if err != nil {
		return cost.PartialSnapshot{}, err
	}

	var collected []cost.Cost
	for _, query := range queries {
		if err := ctx.Err(); err != nil {
			return cost.PartialSnapshot{}, err
		}
		values, err := collector.reader.ReadCosts(ctx, query)
		if err != nil {
			return cost.PartialSnapshot{}, fmt.Errorf("collect %s service cost: %w", query.Window, err)
		}
		values, err = basecollector.LimitDimensions(values, collector.seriesLimit, OtherService, collector.observers...)
		if err != nil {
			return cost.PartialSnapshot{}, fmt.Errorf("limit %s service cost: %w", query.Window, err)
		}
		collected = append(collected, values...)
	}
	return cost.NewSnapshot(collected, nil), nil
}
