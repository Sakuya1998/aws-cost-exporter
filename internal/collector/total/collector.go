// Package total collects ungrouped daily and month-to-date costs.
package total

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
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
	reader Reader
}

// New validates dependencies and constructs a total collector.
func New(reader Reader) (*Collector, error) {
	if reader == nil {
		return nil, ErrNilReader
	}
	return &Collector{reader: reader}, nil
}

// Name returns the stable collector identifier.
func (collector *Collector) Name() string {
	return Name
}

// Collect retrieves daily and month-to-date totals as one publishable result.
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
		{Period: day, Window: cost.WindowDaily, GroupBy: cost.DimensionTotal},
		{Period: monthToDate, Window: cost.WindowMonthToDate, GroupBy: cost.DimensionTotal},
	}

	var collected []cost.Cost
	for _, query := range queries {
		values, err := collector.reader.ReadCosts(ctx, query)
		if err != nil {
			return cost.PartialSnapshot{}, fmt.Errorf("collect %s total cost: %w", query.Window, err)
		}
		collected = append(collected, values...)
	}
	return cost.NewSnapshot(collected, nil), nil
}
