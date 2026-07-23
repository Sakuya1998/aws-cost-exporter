package snapshot

import (
	"errors"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/budget"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/organization"
)

func TestSnapshotCopiesSortsTraversesAndCounts(t *testing.T) {
	day := cost.DayContaining(time.Now())
	dimension, _ := cost.NewDimension(cost.DimensionTotal, "")
	one, _ := cost.NewMoney(1, "USD")
	two, _ := cost.NewMoney(2, "USD")
	costs := []cost.Cost{{Target: "b", Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Period: day, Dimension: dimension, Amount: two}, {Target: "a", Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Period: day, Dimension: dimension, Amount: one}}
	actual := one
	value := New(costs, []cost.Forecast{{Target: "a", Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Period: day, Mean: one, LowerBound: one, UpperBound: two}}, []budget.Budget{{Target: "a", Name: "Monthly", Type: "COST", TimeUnit: "MONTHLY", Limit: two, Actual: actual, HasActual: true}}, []organization.Account{{Target: "a", AccountID: "111111111111", Name: "one", Status: "ACTIVE"}})
	costs[0].Target = "changed"
	if got := value.Costs(); len(got) != 2 || got[0].Target != "a" {
		t.Fatalf("costs=%#v", got)
	}
	if len(value.Forecasts()) != 1 || len(value.Budgets()) != 1 || len(value.Accounts()) != 1 || value.SeriesCount() != 8 {
		t.Fatalf("snapshot count=%d", value.SeriesCount())
	}
	visited := 0
	value.ForEachCost(func(cost.Cost) { visited++ })
	value.ForEachForecast(func(cost.Forecast) { visited++ })
	value.ForEachBudget(func(budget.Budget) { visited++ })
	value.ForEachAccount(func(organization.Account) { visited++ })
	if visited != 5 {
		t.Fatalf("visited=%d", visited)
	}
	value.ForEachCost(nil)
	value.ForEachForecast(nil)
	value.ForEachBudget(nil)
	value.ForEachAccount(nil)
}

func TestMergeAndValidation(t *testing.T) {
	day := cost.DayContaining(time.Now())
	dimension, _ := cost.NewDimension(cost.DimensionTotal, "")
	money, _ := cost.NewMoney(1, "USD")
	partial := New([]cost.Cost{{Target: "a", Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Period: day, Dimension: dimension, Amount: money}}, nil, nil, nil)
	merged := Merge(partial, New(nil, nil, nil, nil))
	if len(merged.Costs()) != 1 {
		t.Fatal("merge lost values")
	}
	if err := merged.ValidatePartial("a"); err != nil {
		t.Fatal(err)
	}
	if err := merged.ValidatePartial("b"); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("target validation=%v", err)
	}
	if err := merged.ValidateUnique(); err != nil {
		t.Fatal(err)
	}
	duplicate := Merge(partial, partial)
	if err := duplicate.ValidateUnique(); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("duplicate validation=%v", err)
	}
}
