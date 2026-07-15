package costexplorer

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

const metricUnblendedCost = "UnblendedCost"

// ErrInvalidResponse indicates an incomplete or unexpected AWS result shape.
var ErrInvalidResponse = errors.New("invalid Cost Explorer response")

// MapUsage converts complete AWS results into provider-independent cost values.
func MapUsage(results []cetypes.ResultByTime, query ports.CostQuery, costMetric string) ([]cost.Cost, error) {
	if costMetric == "" {
		return nil, fmt.Errorf("%w: cost metric must not be empty", ErrInvalidResponse)
	}
	if query.GroupBy != cost.DimensionTotal &&
		query.GroupBy != cost.DimensionService &&
		query.GroupBy != cost.DimensionRegion &&
		query.GroupBy != cost.DimensionAccount {
		return nil, fmt.Errorf("%w: unsupported group", ErrInvalidResponse)
	}
	mapped := make([]cost.Cost, 0, len(results))
	for resultIndex, result := range results {
		period, err := mapPeriod(result.TimePeriod)
		if err != nil {
			return nil, fmt.Errorf("map result %d: %w", resultIndex, err)
		}
		if query.GroupBy == cost.DimensionTotal {
			dimension, _ := cost.NewDimension(cost.DimensionTotal, "")
			entry, err := mapCost(period, query.Window, dimension, result.Total, costMetric)
			if err != nil {
				return nil, fmt.Errorf("map result %d: %w", resultIndex, err)
			}
			mapped = append(mapped, entry)
			continue
		}
		for groupIndex, group := range result.Groups {
			if len(group.Keys) != 1 {
				return nil, fmt.Errorf("%w: result %d group %d must have one key", ErrInvalidResponse, resultIndex, groupIndex)
			}
			dimension, err := cost.NewDimension(query.GroupBy, group.Keys[0])
			if err != nil {
				return nil, fmt.Errorf("map result %d group %d: %w", resultIndex, groupIndex, err)
			}
			entry, err := mapCost(period, query.Window, dimension, group.Metrics, costMetric)
			if err != nil {
				return nil, fmt.Errorf("map result %d group %d: %w", resultIndex, groupIndex, err)
			}
			mapped = append(mapped, entry)
		}
	}
	if query.Window != cost.WindowMonthToDate {
		return mapped, nil
	}
	return aggregateMonthToDate(mapped, query.Period)
}

type aggregateKey struct {
	kind     cost.DimensionKind
	value    string
	currency string
}

// aggregateMonthToDate sums daily Cost Explorer rows into one observation per
// dimension and currency for month-to-date export.
func aggregateMonthToDate(costs []cost.Cost, period cost.Period) ([]cost.Cost, error) {
	if len(costs) == 0 {
		return nil, nil
	}
	totals := make(map[aggregateKey]cost.Cost, len(costs))
	for _, entry := range costs {
		key := aggregateKey{
			kind:     entry.Dimension.Kind(),
			value:    entry.Dimension.Value(),
			currency: entry.Amount.Currency(),
		}
		existing, exists := totals[key]
		if !exists {
			totals[key] = cost.Cost{
				Window:    cost.WindowMonthToDate,
				Period:    period,
				Dimension: entry.Dimension,
				Amount:    entry.Amount,
			}
			continue
		}
		sum, err := existing.Amount.Add(entry.Amount)
		if err != nil {
			return nil, err
		}
		existing.Amount = sum
		totals[key] = existing
	}
	keys := make([]aggregateKey, 0, len(totals))
	for key := range totals {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		if keys[left].kind != keys[right].kind {
			return keys[left].kind < keys[right].kind
		}
		if keys[left].value != keys[right].value {
			return keys[left].value < keys[right].value
		}
		return keys[left].currency < keys[right].currency
	})
	aggregated := make([]cost.Cost, 0, len(keys))
	for _, key := range keys {
		aggregated = append(aggregated, totals[key])
	}
	return aggregated, nil
}

// mapPeriod converts an AWS date interval to an exclusive UTC domain period.
func mapPeriod(interval *cetypes.DateInterval) (cost.Period, error) {
	if interval == nil || interval.Start == nil || interval.End == nil {
		return cost.Period{}, ErrInvalidResponse
	}
	start, startErr := time.Parse(time.DateOnly, aws.ToString(interval.Start))
	end, endErr := time.Parse(time.DateOnly, aws.ToString(interval.End))
	if startErr != nil || endErr != nil {
		return cost.Period{}, fmt.Errorf("%w: invalid date interval", ErrInvalidResponse)
	}
	period, err := cost.NewPeriod(start, end)
	if err != nil {
		return cost.Period{}, fmt.Errorf("%w: invalid period", ErrInvalidResponse)
	}
	return period, nil
}

// mapCost converts one AWS metric map to a domain cost.
func mapCost(period cost.Period, window cost.Window, dimension cost.Dimension, metrics map[string]cetypes.MetricValue, costMetric string) (cost.Cost, error) {
	metric, exists := metrics[costMetric]
	if !exists {
		return cost.Cost{}, fmt.Errorf("%w: missing %s", ErrInvalidResponse, costMetric)
	}
	amount, err := cost.ParseMoney(aws.ToString(metric.Amount), aws.ToString(metric.Unit))
	if err != nil {
		return cost.Cost{}, err
	}
	return cost.Cost{Window: window, Period: period, Dimension: dimension, Amount: amount}, nil
}
