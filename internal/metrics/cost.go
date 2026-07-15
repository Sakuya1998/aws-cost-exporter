// Package metrics exposes immutable cost snapshots to Prometheus.
package metrics

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

// SnapshotReader supplies a lock-free snapshot with unique exported label sets.
type SnapshotReader interface {
	Snapshot() cost.Snapshot
}

// ErrNilSnapshotReader indicates missing metric data storage.
var ErrNilSnapshotReader = errors.New("metrics snapshot reader must not be nil")

type costDefinition struct {
	window cost.Window
	kind   cost.DimensionKind
	desc   *prometheus.Desc
}

type costDefinitionKey struct {
	window cost.Window
	kind   cost.DimensionKind
}

// CostCollector maps domain snapshots to stable business metric families.
type CostCollector struct {
	store     SnapshotReader
	costs     []costDefinition
	indexed   map[costDefinitionKey]*prometheus.Desc
	forecasts [3]*prometheus.Desc
}

// NewCostCollector validates dependencies and defines the metric contract.
func NewCostCollector(store SnapshotReader) (*CostCollector, error) {
	if store == nil {
		return nil, ErrNilSnapshotReader
	}
	collector := &CostCollector{
		store: store,
		costs: []costDefinition{
			{cost.WindowDaily, cost.DimensionTotal, newDesc("daily_amount", "Current UTC billing day accumulated cost.", "")},
			{cost.WindowMonthToDate, cost.DimensionTotal, newDesc("month_to_date_amount", "Current UTC month-to-date accumulated cost.", "")},
			{cost.WindowDaily, cost.DimensionService, newDesc("service_daily_amount", "Current UTC billing day cost by AWS service.", "aws_service")},
			{cost.WindowMonthToDate, cost.DimensionService, newDesc("service_month_to_date_amount", "Current UTC month-to-date cost by AWS service.", "aws_service")},
			{cost.WindowDaily, cost.DimensionRegion, newDesc("region_daily_amount", "Current UTC billing day cost by AWS region.", "aws_region")},
			{cost.WindowMonthToDate, cost.DimensionRegion, newDesc("region_month_to_date_amount", "Current UTC month-to-date cost by AWS region.", "aws_region")},
			{cost.WindowDaily, cost.DimensionAccount, newDesc("account_daily_amount", "Current UTC billing day cost by linked account.", "linked_account_id")},
			{cost.WindowMonthToDate, cost.DimensionAccount, newDesc("account_month_to_date_amount", "Current UTC month-to-date cost by linked account.", "linked_account_id")},
		},
		forecasts: [3]*prometheus.Desc{
			newDesc("month_forecast_mean_amount", "Forecast mean for the remaining current UTC month, including today.", ""),
			newDesc("month_forecast_lower_bound_amount", "Forecast lower bound for the remaining current UTC month, including today.", ""),
			newDesc("month_forecast_upper_bound_amount", "Forecast upper bound for the remaining current UTC month, including today.", ""),
		},
	}
	collector.indexed = make(map[costDefinitionKey]*prometheus.Desc, len(collector.costs))
	for _, definition := range collector.costs {
		collector.indexed[costDefinitionKey{definition.window, definition.kind}] = definition.desc
	}
	return collector, nil
}

// Describe sends all fixed business metric descriptors.
func (collector *CostCollector) Describe(output chan<- *prometheus.Desc) {
	for _, definition := range collector.costs {
		output <- definition.desc
	}
	for _, description := range collector.forecasts {
		output <- description
	}
}

// Collect translates one atomic snapshot read into Prometheus gauges.
func (collector *CostCollector) Collect(output chan<- prometheus.Metric) {
	snapshot := collector.store.Snapshot()
	snapshot.ForEachCost(func(value cost.Cost) {
		definition := collector.definition(value.Window, value.Dimension.Kind())
		if definition == nil {
			return
		}
		labels := []string{value.Amount.Currency()}
		if value.Dimension.Kind() != cost.DimensionTotal {
			labels = append(labels, value.Dimension.Value())
		}
		output <- prometheus.MustNewConstMetric(
			definition, prometheus.GaugeValue, value.Amount.Amount(), labels...,
		)
	})
	snapshot.ForEachForecast(func(forecast cost.Forecast) {
		amounts := [3]cost.Money{forecast.Mean, forecast.LowerBound, forecast.UpperBound}
		for index, amount := range amounts {
			output <- prometheus.MustNewConstMetric(
				collector.forecasts[index], prometheus.GaugeValue,
				amount.Amount(), amount.Currency(),
			)
		}
	})
}

func (collector *CostCollector) definition(window cost.Window, kind cost.DimensionKind) *prometheus.Desc {
	return collector.indexed[costDefinitionKey{window, kind}]
}

func newDesc(name, help, dimension string) *prometheus.Desc {
	labels := []string{"currency"}
	if dimension != "" {
		labels = append(labels, dimension)
	}
	return prometheus.NewDesc("aws_cost_"+name, help, labels, nil)
}

var _ prometheus.Collector = (*CostCollector)(nil)
