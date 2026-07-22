// Package snapshot owns the immutable multi-domain application snapshot.
package snapshot

import (
	"errors"
	"fmt"
	"sort"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/anomaly"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/budget"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/commitment"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/organization"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/tagcost"
)

// ErrInvalidSnapshot indicates target contamination or duplicate metric labels.
var ErrInvalidSnapshot = errors.New("invalid aggregate snapshot")

// Snapshot is an immutable, deterministically ordered aggregate result.
type Snapshot struct {
	costs       []cost.Cost
	forecasts   []cost.Forecast
	budgets     []budget.Budget
	accounts    []organization.Account
	commitments []commitment.Summary
	anomalies   []anomaly.Summary
	tagCosts    []tagcost.Cost
}

// PartialSnapshot is the strongly typed result returned by one collector.
type PartialSnapshot = Snapshot

// New copies and deterministically orders all supplied domain records.
func New(costs []cost.Cost, forecasts []cost.Forecast, budgets []budget.Budget, accounts []organization.Account) Snapshot {
	return NewWithData(costs, forecasts, budgets, accounts, nil, nil, nil)
}

// NewWithData copies and deterministically orders all v0.3 domain records.
func NewWithData(costs []cost.Cost, forecasts []cost.Forecast, budgets []budget.Budget, accounts []organization.Account, commitments []commitment.Summary, anomalies []anomaly.Summary, tags []tagcost.Cost) Snapshot {
	result := Snapshot{
		costs: append([]cost.Cost(nil), costs...), forecasts: append([]cost.Forecast(nil), forecasts...),
		budgets: append([]budget.Budget(nil), budgets...), accounts: append([]organization.Account(nil), accounts...),
		commitments: append([]commitment.Summary(nil), commitments...), anomalies: append([]anomaly.Summary(nil), anomalies...), tagCosts: append([]tagcost.Cost(nil), tags...),
	}
	for index := range result.costs {
		result.costs[index].Provider = cost.NormalizeProvider(result.costs[index].Provider)
		result.costs[index].Basis = cost.NormalizeBasis(result.costs[index].Basis)
	}
	for index := range result.forecasts {
		result.forecasts[index].Provider = cost.NormalizeProvider(result.forecasts[index].Provider)
		result.forecasts[index].Basis = cost.NormalizeBasis(result.forecasts[index].Basis)
	}
	for index := range result.tagCosts {
		result.tagCosts[index].Provider = cost.NormalizeProvider(result.tagCosts[index].Provider)
		result.tagCosts[index].Basis = cost.NormalizeBasis(result.tagCosts[index].Basis)
	}
	sort.SliceStable(result.costs, func(left, right int) bool {
		if result.costs[left].Target != result.costs[right].Target {
			return result.costs[left].Target < result.costs[right].Target
		}
		return cost.Less(result.costs[left], result.costs[right])
	})
	sort.SliceStable(result.forecasts, func(left, right int) bool {
		if result.forecasts[left].Target != result.forecasts[right].Target {
			return result.forecasts[left].Target < result.forecasts[right].Target
		}
		if result.forecasts[left].Provider != result.forecasts[right].Provider {
			return result.forecasts[left].Provider < result.forecasts[right].Provider
		}
		if result.forecasts[left].Basis != result.forecasts[right].Basis {
			return result.forecasts[left].Basis < result.forecasts[right].Basis
		}
		if !result.forecasts[left].Period.Start().Equal(result.forecasts[right].Period.Start()) {
			return result.forecasts[left].Period.Start().Before(result.forecasts[right].Period.Start())
		}
		return result.forecasts[left].Mean.Currency() < result.forecasts[right].Mean.Currency()
	})
	budget.Sort(result.budgets)
	organization.Sort(result.accounts)
	commitment.Sort(result.commitments)
	anomaly.Sort(result.anomalies)
	tagcost.Sort(result.tagCosts)
	return result
}

// Merge combines partials without mutating their storage.
func Merge(parts ...PartialSnapshot) Snapshot {
	var costs []cost.Cost
	var forecasts []cost.Forecast
	var budgets []budget.Budget
	var accounts []organization.Account
	var commitments []commitment.Summary
	var anomalies []anomaly.Summary
	var tags []tagcost.Cost
	for _, part := range parts {
		part.ForEachCost(func(value cost.Cost) { costs = append(costs, value) })
		part.ForEachForecast(func(value cost.Forecast) { forecasts = append(forecasts, value) })
		part.ForEachBudget(func(value budget.Budget) { budgets = append(budgets, value) })
		part.ForEachAccount(func(value organization.Account) { accounts = append(accounts, value) })
		part.ForEachCommitment(func(value commitment.Summary) { commitments = append(commitments, value) })
		part.ForEachAnomaly(func(value anomaly.Summary) { anomalies = append(anomalies, value) })
		part.ForEachTagCost(func(value tagcost.Cost) { tags = append(tags, value) })
	}
	return NewWithData(costs, forecasts, budgets, accounts, commitments, anomalies, tags)
}

// Costs returns an isolated compatibility copy.
func (value Snapshot) Costs() []cost.Cost { return append([]cost.Cost(nil), value.costs...) }

// Forecasts returns an isolated compatibility copy.
func (value Snapshot) Forecasts() []cost.Forecast {
	return append([]cost.Forecast(nil), value.forecasts...)
}

// Budgets returns an isolated copy.
func (value Snapshot) Budgets() []budget.Budget {
	return append([]budget.Budget(nil), value.budgets...)
}

// Accounts returns an isolated copy.
func (value Snapshot) Accounts() []organization.Account {
	return append([]organization.Account(nil), value.accounts...)
}

func (value Snapshot) Commitments() []commitment.Summary {
	return append([]commitment.Summary(nil), value.commitments...)
}
func (value Snapshot) Anomalies() []anomaly.Summary {
	return append([]anomaly.Summary(nil), value.anomalies...)
}
func (value Snapshot) TagCosts() []tagcost.Cost {
	return append([]tagcost.Cost(nil), value.tagCosts...)
}

// ForEachCost visits immutable values without copying snapshot storage.
func (value Snapshot) ForEachCost(visit func(cost.Cost)) {
	if visit != nil {
		for _, item := range value.costs {
			visit(item)
		}
	}
}

// ForEachForecast visits immutable values without copying snapshot storage.
func (value Snapshot) ForEachForecast(visit func(cost.Forecast)) {
	if visit != nil {
		for _, item := range value.forecasts {
			visit(item)
		}
	}
}

// ForEachBudget visits immutable values without copying snapshot storage.
func (value Snapshot) ForEachBudget(visit func(budget.Budget)) {
	if visit != nil {
		for _, item := range value.budgets {
			visit(item)
		}
	}
}

// ForEachAccount visits immutable values without copying snapshot storage.
func (value Snapshot) ForEachAccount(visit func(organization.Account)) {
	if visit != nil {
		for _, item := range value.accounts {
			visit(item)
		}
	}
}

func (value Snapshot) ForEachCommitment(visit func(commitment.Summary)) {
	if visit != nil {
		for _, item := range value.commitments {
			visit(item)
		}
	}
}
func (value Snapshot) ForEachAnomaly(visit func(anomaly.Summary)) {
	if visit != nil {
		for _, item := range value.anomalies {
			visit(item)
		}
	}
}
func (value Snapshot) ForEachTagCost(visit func(tagcost.Cost)) {
	if visit != nil {
		for _, item := range value.tagCosts {
			visit(item)
		}
	}
}

// SeriesCount returns the business series owned by this partial.
func (value Snapshot) SeriesCount() int {
	count := len(value.costs) + 3*len(value.forecasts) + len(value.accounts) + len(value.tagCosts)
	for _, item := range value.commitments {
		if item.HasUtilization {
			count++
		}
		if item.HasCoverage {
			count++
		}
		if item.HasUnusedHours {
			count++
		}
		if item.HasCoveredSpend {
			count++
		}
		if item.HasOnDemandCost {
			count++
		}
		if item.HasNetSavings {
			count++
		}
	}
	for _, item := range value.anomalies {
		count += 2
		if item.HasImpact {
			count++
		}
		if !item.LastDetected.IsZero() {
			count++
		}
	}
	for _, item := range value.budgets {
		count++
		if item.HasActual {
			count++
		}
		if item.HasForecasted {
			count++
		}
	}
	return count
}

// ValidatePartial ensures one collector cannot publish records for another target.
func (value Snapshot) ValidatePartial(target identity.TargetID) error {
	valid := true
	value.ForEachCost(func(item cost.Cost) { valid = valid && item.Target == target })
	value.ForEachForecast(func(item cost.Forecast) { valid = valid && item.Target == target })
	value.ForEachBudget(func(item budget.Budget) { valid = valid && item.Target == target })
	value.ForEachAccount(func(item organization.Account) { valid = valid && item.Target == target })
	value.ForEachCommitment(func(item commitment.Summary) { valid = valid && item.Target == target })
	value.ForEachAnomaly(func(item anomaly.Summary) { valid = valid && item.Target == target })
	value.ForEachTagCost(func(item tagcost.Cost) { valid = valid && item.Target == target })
	if !valid {
		return fmt.Errorf("%w: target mismatch", ErrInvalidSnapshot)
	}
	return nil
}

// ValidateUnique rejects duplicate Prometheus label sets before publication.
func (value Snapshot) ValidateUnique() error {
	seen := make(map[string]struct{}, value.SeriesCount())
	add := func(key string) bool {
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = struct{}{}
		return true
	}
	valid := true
	value.ForEachCost(func(item cost.Cost) {
		valid = valid && add(fmt.Sprintf("cost|%s|%s|%s|%s|%s|%s|%s", item.Target, item.Provider, item.Basis, item.Window, item.Dimension.Kind(), item.Dimension.Value(), item.Amount.Currency()))
	})
	value.ForEachForecast(func(item cost.Forecast) {
		valid = valid && add(fmt.Sprintf("forecast|%s|%s|%s|%s", item.Target, item.Provider, item.Basis, item.Mean.Currency()))
	})
	value.ForEachBudget(func(item budget.Budget) {
		prefix := fmt.Sprintf("budget|%s|%s|%s|%s|", item.Target, item.Name, item.Type, item.TimeUnit)
		valid = valid && add(prefix+"limit|"+item.Limit.Currency())
		if item.HasActual {
			valid = valid && add(prefix+"actual|"+item.Actual.Currency())
		}
		if item.HasForecasted {
			valid = valid && add(prefix+"forecasted|"+item.Forecasted.Currency())
		}
	})
	value.ForEachAccount(func(item organization.Account) {
		valid = valid && add(fmt.Sprintf("account|%s|%s|%s|%s", item.Target, item.AccountID, item.Name, item.Status))
	})
	value.ForEachTagCost(func(item tagcost.Cost) {
		valid = valid && add(fmt.Sprintf("tag|%s|%s|%s|%s|%s|%s|%s", item.Target, item.Provider, item.Basis, item.Window, item.TagKey, item.TagValue, item.Amount.Currency()))
	})
	value.ForEachCommitment(func(item commitment.Summary) {
		prefix := fmt.Sprintf("commitment|%s|%s|%s|", item.Target, item.Type, item.TimeUnit)
		if item.HasUtilization {
			valid = valid && add(prefix+"utilization")
		}
		if item.HasCoverage {
			valid = valid && add(prefix+"coverage")
		}
		if item.HasUnusedHours {
			valid = valid && add(prefix+"unused_hours")
		}
		if item.HasCoveredSpend {
			valid = valid && add(prefix+"covered_spend|"+item.CoveredSpend.Currency())
		}
		if item.HasOnDemandCost {
			valid = valid && add(prefix+"on_demand_cost|"+item.OnDemandCost.Currency())
		}
		if item.HasNetSavings {
			valid = valid && add(prefix+"net_savings|"+item.NetSavings.Currency())
		}
	})
	value.ForEachAnomaly(func(item anomaly.Summary) {
		prefix := fmt.Sprintf("anomaly|%s|", item.Target)
		for _, suffix := range []string{"active", "count", "last_detected"} {
			valid = valid && add(prefix+suffix)
		}
		if item.HasImpact {
			valid = valid && add(prefix+"impact|"+item.Impact.Currency())
		}
	})
	if !valid {
		return fmt.Errorf("%w: duplicate metric label set", ErrInvalidSnapshot)
	}
	return nil
}
