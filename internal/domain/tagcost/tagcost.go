// Package tagcost contains bounded tag cost observations.
package tagcost

import (
	"sort"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
)

type Cost struct {
	Target   identity.TargetID
	Provider cost.Provider
	Basis    cost.Basis
	Window   cost.Window
	TagKey   string
	TagValue string
	Amount   cost.Money
}

func Sort(values []Cost) {
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].Target != values[j].Target {
			return values[i].Target < values[j].Target
		}
		if values[i].Provider != values[j].Provider {
			return values[i].Provider < values[j].Provider
		}
		if values[i].Basis != values[j].Basis {
			return values[i].Basis < values[j].Basis
		}
		if values[i].Window != values[j].Window {
			return values[i].Window < values[j].Window
		}
		if values[i].TagKey != values[j].TagKey {
			return values[i].TagKey < values[j].TagKey
		}
		if values[i].TagValue != values[j].TagValue {
			return values[i].TagValue < values[j].TagValue
		}
		return values[i].Amount.Currency() < values[j].Amount.Currency()
	})
}
