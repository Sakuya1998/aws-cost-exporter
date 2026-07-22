package costexplorer

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

// ErrNilForecastAPI indicates a missing Cost Explorer API dependency.
var ErrNilForecastAPI = errors.New("cost explorer forecast API must not be nil")

// ForecastAPI is the SDK surface required by the forecast adapter.
type ForecastAPI interface {
	GetCostForecast(context.Context, *awscostexplorer.GetCostForecastInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostForecastOutput, error)
}

// ForecastAdapter retrieves normalized monthly forecasts.
type ForecastAdapter struct {
	api ForecastAPI
}

// NewForecastAdapter validates and constructs a forecast adapter.
func NewForecastAdapter(api ForecastAPI) (*ForecastAdapter, error) {
	if api == nil {
		return nil, ErrNilForecastAPI
	}
	return &ForecastAdapter{api: api}, nil
}

// ReadForecast requests one monthly result and maps it to the domain.
func (adapter *ForecastAdapter) ReadForecast(ctx context.Context, query ports.ForecastQuery) (cost.Forecast, error) {
	metric, err := forecastMetric(query.Basis)
	if err != nil {
		return cost.Forecast{}, err
	}
	output, err := adapter.api.GetCostForecast(ctx, &awscostexplorer.GetCostForecastInput{
		Granularity:             cetypes.GranularityMonthly,
		Metric:                  metric,
		PredictionIntervalLevel: aws.Int32(int32(query.PredictionInterval)), // #nosec G115 -- the collector bounds this internal value to 80..99.
		Filter: dimensionFilter(
			query.LinkedAccountIDs, query.Services, query.Regions,
		),
		TimePeriod: &cetypes.DateInterval{
			Start: aws.String(query.Period.Start().Format("2006-01-02")),
			End:   aws.String(query.Period.End().Format("2006-01-02")),
		},
	})
	if err != nil {
		return cost.Forecast{}, ClassifyError(err)
	}
	return MapForecast(output, query)
}

func forecastMetric(basis cost.Basis) (cetypes.Metric, error) {
	switch cost.NormalizeBasis(basis) {
	case cost.BasisUnblended:
		return cetypes.MetricUnblendedCost, nil
	case cost.BasisAmortized:
		return cetypes.MetricAmortizedCost, nil
	case cost.BasisNet:
		return cetypes.MetricNetUnblendedCost, nil
	default:
		return "", fmt.Errorf("%w: unsupported forecast cost basis", ErrInvalidResponse)
	}
}

// MapForecast validates one AWS monthly forecast result.
func MapForecast(output *awscostexplorer.GetCostForecastOutput, query ports.ForecastQuery) (cost.Forecast, error) {
	if output == nil || output.Total == nil || len(output.ForecastResultsByTime) != 1 {
		return cost.Forecast{}, fmt.Errorf("%w: expected one forecast result", ErrInvalidResponse)
	}
	result := output.ForecastResultsByTime[0]
	period, err := mapPeriod(result.TimePeriod)
	if err != nil {
		return cost.Forecast{}, err
	}
	// Cost Explorer may normalize a monthly bucket's start to the first day;
	// the domain forecast retains the requested remaining-month window.
	month := cost.MonthContaining(query.Period.Start())
	validStart := period.Start().Equal(query.Period.Start()) || period.Start().Equal(month.Start())
	if !validStart || !period.End().Equal(query.Period.End()) {
		return cost.Forecast{}, fmt.Errorf("%w: forecast period mismatch", ErrInvalidResponse)
	}
	currency := aws.ToString(output.Total.Unit)
	mean, err := cost.ParseMoney(aws.ToString(result.MeanValue), currency)
	if err != nil {
		return cost.Forecast{}, fmt.Errorf("map forecast mean: %w", err)
	}
	lower, err := cost.ParseMoney(aws.ToString(result.PredictionIntervalLowerBound), currency)
	if err != nil {
		return cost.Forecast{}, fmt.Errorf("map forecast lower bound: %w", err)
	}
	upper, err := cost.ParseMoney(aws.ToString(result.PredictionIntervalUpperBound), currency)
	if err != nil {
		return cost.Forecast{}, fmt.Errorf("map forecast upper bound: %w", err)
	}
	if lower.Amount() > mean.Amount() || mean.Amount() > upper.Amount() {
		return cost.Forecast{}, fmt.Errorf("%w: prediction bounds are unordered", ErrInvalidResponse)
	}
	return cost.Forecast{
		Provider: cost.ProviderCostExplorer, Basis: cost.NormalizeBasis(query.Basis), Period: query.Period, Mean: mean, LowerBound: lower, UpperBound: upper,
	}, nil
}
