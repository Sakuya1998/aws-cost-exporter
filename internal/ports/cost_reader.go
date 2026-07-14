package ports

import (
	"context"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

// CostQuery describes one GetCostAndUsage request without AWS SDK types.
type CostQuery struct {
	Period           cost.Period
	Window           cost.Window
	GroupBy          cost.DimensionKind
	LinkedAccountIDs []string
	Services         []string
	Regions          []string
}

// ForecastQuery describes one GetCostForecast request without AWS SDK types.
type ForecastQuery struct {
	Period             cost.Period
	PredictionInterval int
	LinkedAccountIDs   []string
	Services           []string
	Regions            []string
}

// CostReader retrieves normalized cost data from a billing provider.
type CostReader interface {
	// ReadCosts returns all pages for one cost query or no publishable result.
	ReadCosts(ctx context.Context, query CostQuery) ([]cost.Cost, error)
	// ReadForecast returns prediction bounds for one forecast query.
	ReadForecast(ctx context.Context, query ForecastQuery) (cost.Forecast, error)
}
