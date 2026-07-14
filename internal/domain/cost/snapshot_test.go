package cost_test

import (
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

// TestNewSnapshotSortsAndCopiesRecords verifies deterministic output and
// prevents callers from mutating published snapshot data.
func TestNewSnapshotSortsAndCopiesRecords(t *testing.T) {
	t.Parallel()

	period := cost.DayContaining(time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC))
	alpha, _ := cost.NewDimension(cost.DimensionService, "Alpha")
	beta, _ := cost.NewDimension(cost.DimensionService, "Beta")
	one, _ := cost.NewMoney(1, "USD")
	two, _ := cost.NewMoney(2, "USD")

	costs := []cost.Cost{
		{Window: cost.WindowDaily, Period: period, Dimension: beta, Amount: two},
		{Window: cost.WindowDaily, Period: period, Dimension: alpha, Amount: one},
	}
	forecasts := []cost.Forecast{{
		Period: period, Mean: two, LowerBound: one, UpperBound: two,
	}}
	snapshot := cost.NewSnapshot(costs, forecasts)

	costs[0].Amount = one
	forecasts[0].Mean = one
	gotCosts := snapshot.Costs()
	gotForecasts := snapshot.Forecasts()
	if gotCosts[0].Dimension.Value() != "Alpha" || gotCosts[1].Amount.Amount() != 2 {
		t.Fatalf("Costs() returned unsorted or mutated data: %+v", gotCosts)
	}
	if gotForecasts[0].Mean.Amount() != 2 {
		t.Fatalf("Forecasts()[0].Mean = %v, want 2", gotForecasts[0].Mean.Amount())
	}

	gotCosts[0].Amount = two
	if snapshot.Costs()[0].Amount.Amount() != 1 {
		t.Fatal("Costs() exposed mutable snapshot storage")
	}
}

// TestMergeSnapshotsPreservesTotal verifies combining collector results does
// not lose or duplicate monetary records.
func TestMergeSnapshotsPreservesTotal(t *testing.T) {
	t.Parallel()

	period := cost.MonthContaining(time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC))
	service, _ := cost.NewDimension(cost.DimensionService, "Amazon EC2")
	region, _ := cost.NewDimension(cost.DimensionRegion, "us-east-1")
	firstAmount, _ := cost.NewMoney(1.25, "USD")
	secondAmount, _ := cost.NewMoney(2.75, "USD")

	first := cost.NewSnapshot([]cost.Cost{{
		Window: cost.WindowMonthToDate, Period: period, Dimension: service, Amount: firstAmount,
	}}, nil)
	second := cost.NewSnapshot([]cost.Cost{{
		Window: cost.WindowMonthToDate, Period: period, Dimension: region, Amount: secondAmount,
	}}, nil)

	merged := cost.MergeSnapshots(first, second)
	var total float64
	for _, entry := range merged.Costs() {
		total += entry.Amount.Amount()
	}

	if len(merged.Costs()) != 2 || total != 4 {
		t.Fatalf("MergeSnapshots() produced %d costs totaling %v, want 2 costs totaling 4", len(merged.Costs()), total)
	}
}
