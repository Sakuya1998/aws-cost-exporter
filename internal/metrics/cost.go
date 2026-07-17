// Package metrics exposes immutable application snapshots to Prometheus.
package metrics

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/budget"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/organization"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
)

type SnapshotReader interface{ Snapshot() snapshot.Snapshot }

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

// CostCollector maps the unified snapshot to fixed business metric families.
type CostCollector struct {
	store       SnapshotReader
	costs       []costDefinition
	indexed     map[costDefinitionKey]*prometheus.Desc
	forecasts   [3]*prometheus.Desc
	accountInfo *prometheus.Desc
	budgets     [3]*prometheus.Desc
}

func NewCostCollector(store SnapshotReader) (*CostCollector, error) {
	if store == nil {
		return nil, ErrNilSnapshotReader
	}
	collector := &CostCollector{
		store: store,
		costs: []costDefinition{
			{cost.WindowDaily, cost.DimensionTotal, costDesc("daily_amount", "Current UTC billing day accumulated cost.", "")},
			{cost.WindowMonthToDate, cost.DimensionTotal, costDesc("month_to_date_amount", "Current UTC month-to-date accumulated cost.", "")},
			{cost.WindowDaily, cost.DimensionService, costDesc("service_daily_amount", "Current UTC billing day cost by AWS service.", "aws_service")},
			{cost.WindowMonthToDate, cost.DimensionService, costDesc("service_month_to_date_amount", "Current UTC month-to-date cost by AWS service.", "aws_service")},
			{cost.WindowDaily, cost.DimensionRegion, costDesc("region_daily_amount", "Current UTC billing day cost by AWS region.", "aws_region")},
			{cost.WindowMonthToDate, cost.DimensionRegion, costDesc("region_month_to_date_amount", "Current UTC month-to-date cost by AWS region.", "aws_region")},
			{cost.WindowDaily, cost.DimensionAccount, costDesc("account_daily_amount", "Current UTC billing day cost by linked account.", "linked_account_id")},
			{cost.WindowMonthToDate, cost.DimensionAccount, costDesc("account_month_to_date_amount", "Current UTC month-to-date cost by linked account.", "linked_account_id")},
		},
		forecasts: [3]*prometheus.Desc{
			costDesc("month_forecast_mean_amount", "Forecast mean for the remaining current UTC month, including today.", ""),
			costDesc("month_forecast_lower_bound_amount", "Forecast lower bound for the remaining current UTC month, including today.", ""),
			costDesc("month_forecast_upper_bound_amount", "Forecast upper bound for the remaining current UTC month, including today.", ""),
		},
		accountInfo: prometheus.NewDesc("aws_cost_account_info", "Non-sensitive AWS Organizations metadata for an exported linked account.", []string{"target", "linked_account_id", "account_name", "account_status"}, nil),
		budgets: [3]*prometheus.Desc{
			budgetDesc("limit_amount", "Configured AWS Budget limit."),
			budgetDesc("actual_amount", "AWS Budget calculated actual spend."),
			budgetDesc("forecasted_amount", "AWS Budget calculated forecasted spend."),
		},
	}
	collector.indexed = make(map[costDefinitionKey]*prometheus.Desc, len(collector.costs))
	for _, definition := range collector.costs {
		collector.indexed[costDefinitionKey{definition.window, definition.kind}] = definition.desc
	}
	return collector, nil
}

func (collector *CostCollector) Describe(output chan<- *prometheus.Desc) {
	for _, definition := range collector.costs {
		output <- definition.desc
	}
	for _, description := range collector.forecasts {
		output <- description
	}
	output <- collector.accountInfo
	for _, description := range collector.budgets {
		output <- description
	}
}

func (collector *CostCollector) Collect(output chan<- prometheus.Metric) {
	value := collector.store.Snapshot()
	value.ForEachCost(func(item cost.Cost) {
		description := collector.indexed[costDefinitionKey{item.Window, item.Dimension.Kind()}]
		if description == nil {
			return
		}
		labels := []string{string(item.Target), item.Amount.Currency()}
		if item.Dimension.Kind() != cost.DimensionTotal {
			labels = append(labels, item.Dimension.Value())
		}
		output <- prometheus.MustNewConstMetric(description, prometheus.GaugeValue, item.Amount.Amount(), labels...)
	})
	value.ForEachForecast(func(item cost.Forecast) {
		amounts := [3]cost.Money{item.Mean, item.LowerBound, item.UpperBound}
		for index, amount := range amounts {
			output <- prometheus.MustNewConstMetric(collector.forecasts[index], prometheus.GaugeValue, amount.Amount(), string(item.Target), amount.Currency())
		}
	})
	value.ForEachAccount(func(item organization.Account) {
		output <- prometheus.MustNewConstMetric(collector.accountInfo, prometheus.GaugeValue, 1, string(item.Target), item.AccountID, item.Name, item.Status)
	})
	value.ForEachBudget(func(item budget.Budget) {
		labels := []string{string(item.Target), item.Name, item.Limit.Currency(), item.Type, item.TimeUnit}
		output <- prometheus.MustNewConstMetric(collector.budgets[0], prometheus.GaugeValue, item.Limit.Amount(), labels...)
		if item.HasActual {
			labels[2] = item.Actual.Currency()
			output <- prometheus.MustNewConstMetric(collector.budgets[1], prometheus.GaugeValue, item.Actual.Amount(), labels...)
		}
		if item.HasForecasted {
			labels[2] = item.Forecasted.Currency()
			output <- prometheus.MustNewConstMetric(collector.budgets[2], prometheus.GaugeValue, item.Forecasted.Amount(), labels...)
		}
	})
}

func costDesc(name, help, dimension string) *prometheus.Desc {
	labels := []string{"target", "currency"}
	if dimension != "" {
		labels = append(labels, dimension)
	}
	return prometheus.NewDesc("aws_cost_"+name, help, labels, nil)
}

func budgetDesc(name, help string) *prometheus.Desc {
	return prometheus.NewDesc("aws_budget_"+name, help, []string{"target", "budget_name", "currency", "budget_type", "time_unit"}, nil)
}

var _ prometheus.Collector = (*CostCollector)(nil)
