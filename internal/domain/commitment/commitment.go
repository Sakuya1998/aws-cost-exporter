// Package commitment contains bounded Savings Plans and Reserved Instance
// utilization/coverage summaries.
package commitment

import (
	"sort"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
)

type Type string

const (
	TypeSavingsPlan Type = "savings_plan"
	TypeReservation Type = "reservation"
)

type Summary struct {
	Target           identity.TargetID
	Type             Type
	TimeUnit         string
	UtilizationRatio float64
	CoverageRatio    float64
	UnusedHours      float64
	CoveredSpend     cost.Money
	OnDemandCost     cost.Money
	NetSavings       cost.Money
	HasUtilization   bool
	HasCoverage      bool
	HasUnusedHours   bool
	HasCoveredSpend  bool
	HasOnDemandCost  bool
	HasNetSavings    bool
}

func (value Type) Valid() bool { return value == TypeSavingsPlan || value == TypeReservation }

func Sort(values []Summary) {
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].Target != values[j].Target {
			return values[i].Target < values[j].Target
		}
		if values[i].Type != values[j].Type {
			return values[i].Type < values[j].Type
		}
		return values[i].TimeUnit < values[j].TimeUnit
	})
}
