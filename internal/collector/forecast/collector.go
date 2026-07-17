// Package forecast collects the current UTC month's remaining forecast.
package forecast

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

// Name is the stable registry and telemetry identifier.
const Name = "forecast"

var (
	// ErrNilReader indicates a missing forecast dependency.
	ErrNilReader = errors.New("forecast reader must not be nil")
	// ErrInvalidPredictionInterval indicates confidence outside AWS limits.
	ErrInvalidPredictionInterval = errors.New("prediction interval must be between 80 and 99")
)

// Reader is the narrow forecast-reading port required by this collector.
type Reader interface {
	ReadForecast(context.Context, ports.ForecastQuery) (cost.Forecast, error)
}

// Collector retrieves one bounded monthly forecast.
type Collector struct {
	id                 identity.CollectorID
	reader             Reader
	predictionInterval int
}

// New validates dependencies and constructs a forecast collector.
func New(reader Reader, predictionInterval int) (*Collector, error) {
	return NewForTarget("default", reader, predictionInterval)
}

// NewForTarget constructs a target-scoped forecast collector.
func NewForTarget(target identity.TargetID, reader Reader, predictionInterval int) (*Collector, error) {
	if reader == nil {
		return nil, ErrNilReader
	}
	if predictionInterval < 80 || predictionInterval > 99 {
		return nil, ErrInvalidPredictionInterval
	}
	return &Collector{id: identity.CollectorID{Target: target, Name: Name}, reader: reader, predictionInterval: predictionInterval}, nil
}

// Name returns the stable collector identifier.
func (collector *Collector) Name() string { return Name }

// ID returns the target-scoped collector identity.
func (collector *Collector) ID() identity.CollectorID { return collector.id }

// Collect retrieves the forecast for the current UTC calendar month.
func (collector *Collector) Collect(ctx context.Context, reference time.Time) (snapshot.PartialSnapshot, error) {
	day := cost.DayContaining(reference)
	month := cost.MonthContaining(reference)
	period, err := cost.NewPeriod(day.Start(), month.End())
	if err != nil {
		return snapshot.PartialSnapshot{}, fmt.Errorf("build forecast period: %w", err)
	}
	value, err := collector.reader.ReadForecast(ctx, ports.ForecastQuery{
		Period: period, PredictionInterval: collector.predictionInterval,
	})
	if err != nil {
		return snapshot.PartialSnapshot{}, fmt.Errorf("collect forecast: %w", err)
	}
	value.Target = collector.id.Target
	return snapshot.New(nil, []cost.Forecast{value}, nil, nil), nil
}
