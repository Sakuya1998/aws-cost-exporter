// Package snapshot owns the immutable multi-domain application snapshot.
package snapshot

import (
	"errors"
	"fmt"
	"sort"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/budget"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/organization"
)

// ErrInvalidSnapshot indicates target contamination or duplicate metric labels.
var ErrInvalidSnapshot = errors.New("invalid aggregate snapshot")

// Snapshot is an immutable, deterministically ordered aggregate result.
type Snapshot struct {
	costs     []cost.Cost
	forecasts []cost.Forecast
	budgets   []budget.Budget
	accounts  []organization.Account
}

// PartialSnapshot is the strongly typed result returned by one collector.
type PartialSnapshot = Snapshot

// New copies and deterministically orders all supplied domain records.
func New(costs []cost.Cost, forecasts []cost.Forecast, budgets []budget.Budget, accounts []organization.Account) Snapshot {
	result := Snapshot{
		costs: append([]cost.Cost(nil), costs...), forecasts: append([]cost.Forecast(nil), forecasts...),
		budgets: append([]budget.Budget(nil), budgets...), accounts: append([]organization.Account(nil), accounts...),
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
		return result.forecasts[left].Period.Start().Before(result.forecasts[right].Period.Start())
	})
	budget.Sort(result.budgets)
	organization.Sort(result.accounts)
	return result
}

// Merge combines partials without mutating their storage.
func Merge(parts ...PartialSnapshot) Snapshot {
	var costs []cost.Cost
	var forecasts []cost.Forecast
	var budgets []budget.Budget
	var accounts []organization.Account
	for _, part := range parts {
		part.ForEachCost(func(value cost.Cost) { costs = append(costs, value) })
		part.ForEachForecast(func(value cost.Forecast) { forecasts = append(forecasts, value) })
		part.ForEachBudget(func(value budget.Budget) { budgets = append(budgets, value) })
		part.ForEachAccount(func(value organization.Account) { accounts = append(accounts, value) })
	}
	return New(costs, forecasts, budgets, accounts)
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

// SeriesCount returns the business series owned by this partial.
func (value Snapshot) SeriesCount() int {
	count := len(value.costs) + 3*len(value.forecasts) + len(value.accounts)
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
		valid = valid && add(fmt.Sprintf("cost|%s|%s|%s|%s|%s", item.Target, item.Window, item.Dimension.Kind(), item.Dimension.Value(), item.Amount.Currency()))
	})
	value.ForEachForecast(func(item cost.Forecast) {
		valid = valid && add(fmt.Sprintf("forecast|%s|%s", item.Target, item.Mean.Currency()))
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
	if !valid {
		return fmt.Errorf("%w: duplicate metric label set", ErrInvalidSnapshot)
	}
	return nil
}
