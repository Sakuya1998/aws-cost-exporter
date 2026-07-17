// Package budget defines immutable AWS Budget observations.
package budget

import (
	"sort"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
)

// Budget is one allowlisted AWS Budget and its optional calculated spend.
type Budget struct {
	Target        identity.TargetID
	Name          string
	Type          string
	TimeUnit      string
	Limit         cost.Money
	Actual        cost.Money
	Forecasted    cost.Money
	HasActual     bool
	HasForecasted bool
}

// Sort orders budgets deterministically for publication and exposition.
func Sort(values []Budget) {
	sort.SliceStable(values, func(left, right int) bool {
		if values[left].Target != values[right].Target {
			return values[left].Target < values[right].Target
		}
		return values[left].Name < values[right].Name
	})
}
