// Package metrics exposes immutable application snapshots to Prometheus.
package metrics

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/anomaly"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/budget"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/commitment"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/organization"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/tagcost"
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
	tagCosts    [2]*prometheus.Desc
	commitments [6]*prometheus.Desc
	anomalies   [4]*prometheus.Desc
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
		tagCosts: [2]*prometheus.Desc{
			tagCostDesc("daily_amount", "Current UTC billing day cost by allowlisted tag."),
			tagCostDesc("month_to_date_amount", "Current UTC month-to-date cost by allowlisted tag."),
		},
		commitments: [6]*prometheus.Desc{
			commitmentDesc("utilization_ratio", "Commitment utilization as a ratio from 0 to 1.", false),
			commitmentDesc("coverage_ratio", "Commitment coverage as a ratio from 0 to 1.", false),
			commitmentDesc("unused_hours", "Unused commitment hours.", false),
			commitmentDesc("covered_spend_amount", "Spend covered by the commitment.", true),
			commitmentDesc("on_demand_cost_amount", "Equivalent on-demand cost.", true),
			commitmentDesc("net_savings_amount", "Net savings from the commitment.", true),
		},
		anomalies: [4]*prometheus.Desc{
			prometheus.NewDesc("aws_cost_anomaly_active", "Whether a cost anomaly is currently active.", []string{"target"}, nil),
			prometheus.NewDesc("aws_cost_anomaly_count", "Cost anomalies in the configured lookback window.", []string{"target"}, nil),
			prometheus.NewDesc("aws_cost_anomaly_impact_amount", "Cumulative impact of cost anomalies.", []string{"target", "currency"}, nil),
			prometheus.NewDesc("aws_cost_anomaly_last_detected_timestamp_seconds", "Latest anomaly detection end timestamp.", []string{"target"}, nil),
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
	for _, description := range collector.tagCosts {
		output <- description
	}
	for _, description := range collector.commitments {
		output <- description
	}
	for _, description := range collector.anomalies {
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
		labels := []string{string(item.Target), string(item.Provider), string(item.Basis), item.Amount.Currency()}
		if item.Dimension.Kind() != cost.DimensionTotal {
			labels = append(labels, item.Dimension.Value())
		}
		output <- prometheus.MustNewConstMetric(description, prometheus.GaugeValue, item.Amount.Amount(), labels...)
	})
	value.ForEachForecast(func(item cost.Forecast) {
		amounts := [3]cost.Money{item.Mean, item.LowerBound, item.UpperBound}
		for index, amount := range amounts {
			output <- prometheus.MustNewConstMetric(collector.forecasts[index], prometheus.GaugeValue, amount.Amount(), string(item.Target), string(item.Provider), string(item.Basis), amount.Currency())
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
	value.ForEachTagCost(func(item tagcost.Cost) {
		index := 0
		if item.Window == cost.WindowMonthToDate {
			index = 1
		}
		output <- prometheus.MustNewConstMetric(collector.tagCosts[index], prometheus.GaugeValue, item.Amount.Amount(), string(item.Target), string(item.Provider), string(item.Basis), item.Amount.Currency(), item.TagKey, item.TagValue)
	})
	value.ForEachCommitment(func(item commitment.Summary) { collectCommitment(collector, output, item) })
	value.ForEachAnomaly(func(item anomaly.Summary) {
		output <- prometheus.MustNewConstMetric(collector.anomalies[0], prometheus.GaugeValue, boolValue(item.Active), string(item.Target))
		output <- prometheus.MustNewConstMetric(collector.anomalies[1], prometheus.GaugeValue, float64(item.Count), string(item.Target))
		if item.HasImpact {
			output <- prometheus.MustNewConstMetric(collector.anomalies[2], prometheus.GaugeValue, item.Impact.Amount(), string(item.Target), item.Impact.Currency())
		}
		if !item.LastDetected.IsZero() {
			output <- prometheus.MustNewConstMetric(collector.anomalies[3], prometheus.GaugeValue, float64(item.LastDetected.Unix()), string(item.Target))
		}
	})
}

func costDesc(name, help, dimension string) *prometheus.Desc {
	labels := []string{"target", "provider", "cost_basis", "currency"}
	if dimension != "" {
		labels = append(labels, dimension)
	}
	return prometheus.NewDesc("aws_cost_"+name, help, labels, nil)
}

func tagCostDesc(name, help string) *prometheus.Desc {
	return prometheus.NewDesc("aws_cost_tag_"+name, help, []string{"target", "provider", "cost_basis", "currency", "tag_key", "tag_value"}, nil)
}
func commitmentDesc(name, help string, currency bool) *prometheus.Desc {
	labels := []string{"target", "commitment_type", "time_unit"}
	if currency {
		labels = append(labels, "currency")
	}
	return prometheus.NewDesc("aws_commitment_"+name, help, labels, nil)
}
func collectCommitment(collector *CostCollector, output chan<- prometheus.Metric, item commitment.Summary) {
	base := []string{string(item.Target), string(item.Type), item.TimeUnit}
	if item.HasUtilization {
		output <- prometheus.MustNewConstMetric(collector.commitments[0], prometheus.GaugeValue, item.UtilizationRatio, base...)
	}
	if item.HasCoverage {
		output <- prometheus.MustNewConstMetric(collector.commitments[1], prometheus.GaugeValue, item.CoverageRatio, base...)
	}
	if item.HasUnusedHours {
		output <- prometheus.MustNewConstMetric(collector.commitments[2], prometheus.GaugeValue, item.UnusedHours, base...)
	}
	if item.HasCoveredSpend {
		output <- prometheus.MustNewConstMetric(collector.commitments[3], prometheus.GaugeValue, item.CoveredSpend.Amount(), append(base, item.CoveredSpend.Currency())...)
	}
	if item.HasOnDemandCost {
		output <- prometheus.MustNewConstMetric(collector.commitments[4], prometheus.GaugeValue, item.OnDemandCost.Amount(), append(base, item.OnDemandCost.Currency())...)
	}
	if item.HasNetSavings {
		output <- prometheus.MustNewConstMetric(collector.commitments[5], prometheus.GaugeValue, item.NetSavings.Amount(), append(base, item.NetSavings.Currency())...)
	}
}
func boolValue(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func budgetDesc(name, help string) *prometheus.Desc {
	return prometheus.NewDesc("aws_budget_"+name, help, []string{"target", "budget_name", "currency", "budget_type", "time_unit"}, nil)
}

var _ prometheus.Collector = (*CostCollector)(nil)
