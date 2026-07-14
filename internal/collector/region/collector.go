// Package region collects region-grouped daily and month-to-date costs.
package region

import (
	"context"
	"errors"
	"fmt"
	"time"

	basecollector "github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

// Name is the stable registry and telemetry identifier.
const Name = "region"

// OtherRegion is the bounded label for aggregated overflow.
const OtherRegion = "__other__"

var (
	// ErrNilReader indicates a missing Cost Explorer dependency.
	ErrNilReader = errors.New("region cost reader must not be nil")
	// ErrInvalidSeriesLimit preserves the collector-specific public contract.
	ErrInvalidSeriesLimit = basecollector.ErrInvalidSeriesLimit
	// ErrMixedCurrency preserves the collector-specific public contract.
	ErrMixedCurrency = basecollector.ErrMixedCurrency
)

// Reader is the narrow cost-reading port required by this collector.
type Reader interface {
	ReadCosts(context.Context, ports.CostQuery) ([]cost.Cost, error)
}

// Collector retrieves region-grouped costs under a bounded series budget.
type Collector struct {
	reader      Reader
	seriesLimit int
	observers   []basecollector.OverflowObserver
}

// New validates dependencies and constructs a region collector.
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

// Collect applies the series budget independently to each metric window.
func (collector *Collector) Collect(
	ctx context.Context,
	reference time.Time,
) (cost.PartialSnapshot, error) {
	day := cost.DayContaining(reference)
	month := cost.MonthContaining(reference)
	monthToDate, err := cost.NewPeriod(month.Start(), day.End())
	if err != nil {
		return cost.PartialSnapshot{}, fmt.Errorf("build month-to-date period: %w", err)
	}
	queries := []ports.CostQuery{
		{Period: day, Window: cost.WindowDaily, GroupBy: cost.DimensionRegion},
		{Period: monthToDate, Window: cost.WindowMonthToDate, GroupBy: cost.DimensionRegion},
	}
	var collected []cost.Cost
	for _, query := range queries {
		values, err := collector.reader.ReadCosts(ctx, query)
		if err != nil {
			return cost.PartialSnapshot{}, fmt.Errorf("collect %s region cost: %w", query.Window, err)
		}
		values, err = basecollector.LimitDimensions(values, collector.seriesLimit, OtherRegion, collector.observers...)
		if err != nil {
			return cost.PartialSnapshot{}, fmt.Errorf("limit %s region cost: %w", query.Window, err)
		}
		collected = append(collected, values...)
	}
	return cost.NewSnapshot(collected, nil), nil
}
