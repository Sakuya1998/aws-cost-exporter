// Package forecast collects the current UTC month's remaining forecast.
package forecast

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
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
	reader             Reader
	predictionInterval int
}

// New validates dependencies and constructs a forecast collector.
func New(reader Reader, predictionInterval int) (*Collector, error) {
	if reader == nil {
		return nil, ErrNilReader
	}
	if predictionInterval < 80 || predictionInterval > 99 {
		return nil, ErrInvalidPredictionInterval
	}
	return &Collector{reader: reader, predictionInterval: predictionInterval}, nil
}

// Name returns the stable collector identifier.
func (collector *Collector) Name() string { return Name }

// Collect retrieves the forecast for the current UTC calendar month.
func (collector *Collector) Collect(ctx context.Context, reference time.Time) (cost.PartialSnapshot, error) {
	month := cost.MonthContaining(reference)
	period, err := cost.NewPeriod(month.Start(), month.End())
	if err != nil {
		return cost.PartialSnapshot{}, fmt.Errorf("build forecast period: %w", err)
	}
	value, err := collector.reader.ReadForecast(ctx, ports.ForecastQuery{
		Period: period, PredictionInterval: collector.predictionInterval,
	})
	if err != nil {
		return cost.PartialSnapshot{}, fmt.Errorf("collect forecast: %w", err)
	}
	return cost.NewSnapshot(nil, []cost.Forecast{value}), nil
}
