package snapshot

import (
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/anomaly"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/commitment"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/tagcost"
)

func TestV03SnapshotTraversalValidationAndSeriesCount(t *testing.T) {
	target := identity.TargetID("payer")
	money, _ := cost.NewMoney(1, "USD")
	dimension, _ := cost.NewDimension(cost.DimensionTotal, "")
	value := NewWithData([]cost.Cost{{Target: target, Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Dimension: dimension, Amount: money}}, []cost.Forecast{{Target: target, Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Mean: money, LowerBound: money, UpperBound: money}}, nil, nil,
		[]commitment.Summary{{Target: target, Type: commitment.TypeSavingsPlan, TimeUnit: "MONTHLY", UtilizationRatio: .5, CoverageRatio: .4, UnusedHours: 1, CoveredSpend: money, OnDemandCost: money, NetSavings: money, HasUtilization: true, HasCoverage: true, HasUnusedHours: true, HasCoveredSpend: true, HasOnDemandCost: true, HasNetSavings: true}},
		[]anomaly.Summary{{Target: target, Active: true, Count: 1, Impact: money, MaxImpact: money, HasImpact: true, HasMaxImpact: true, LastDetected: time.Now()}},
		[]tagcost.Cost{{Target: target, Provider: cost.ProviderCURAthena, Basis: cost.BasisNet, Window: cost.WindowDaily, TagKey: "Environment", TagValue: "prod", Amount: money}})
	if value.SeriesCount() < 10 || value.ValidateUnique() != nil || value.ValidatePartial(target) != nil {
		t.Fatalf("snapshot invalid: series=%d", value.SeriesCount())
	}
	value.ForEachCost(func(cost.Cost) {})
	value.ForEachForecast(func(cost.Forecast) {})
	value.ForEachCommitment(func(commitment.Summary) {})
	value.ForEachAnomaly(func(anomaly.Summary) {})
	value.ForEachTagCost(func(tagcost.Cost) {})
	if value.ValidatePartial("other") == nil {
		t.Fatal("accepted target contamination")
	}
	invalidIdentity := NewWithData([]cost.Cost{{Target: target, Window: cost.WindowDaily, Dimension: dimension, Amount: money}}, nil, nil, nil, nil, nil, nil)
	if err := invalidIdentity.ValidatePartial(target); err == nil || err.Error() != "invalid aggregate snapshot: target, provider, or cost basis mismatch" {
		t.Fatalf("ValidatePartial()=%v", err)
	}
	if err := invalidIdentity.ValidateUnique(); err == nil || err.Error() != "invalid aggregate snapshot: invalid provider or cost basis" {
		t.Fatalf("ValidateUnique()=%v", err)
	}
	duplicate := NewWithData([]cost.Cost{{Target: target, Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Dimension: dimension, Amount: money}, {Target: target, Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Dimension: dimension, Amount: money}}, nil, nil, nil, nil, nil, nil)
	if duplicate.ValidateUnique() == nil {
		t.Fatal("accepted duplicate labels")
	}
}
