package collector_test

import (
	"errors"
	"testing"

	basecollector "github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

func TestLimitDimensionsPreservesTieBreakOrder(t *testing.T) {
	service, _ := cost.NewDimension(cost.DimensionService, "Amazon S3")
	ec2, _ := cost.NewDimension(cost.DimensionService, "Amazon EC2")
	rds, _ := cost.NewDimension(cost.DimensionService, "Amazon RDS")
	values := []cost.Cost{
		{Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Dimension: service, Amount: mustMoney(t, 10, "USD")},
		{Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Dimension: ec2, Amount: mustMoney(t, 10, "USD")},
		{Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Dimension: rds, Amount: mustMoney(t, 5, "USD")},
	}
	limited, err := basecollector.LimitDimensions(values, 2, "__other__")
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 || limited[0].Dimension.Value() != "Amazon EC2" ||
		limited[1].Dimension.Value() != "__other__" {
		t.Fatalf("limited = %#v, want EC2 plus overflow", limited)
	}
}

func TestLimitDimensionsAggregatesOverflow(t *testing.T) {
	ec2, _ := cost.NewDimension(cost.DimensionService, "Amazon EC2")
	s3, _ := cost.NewDimension(cost.DimensionService, "Amazon S3")
	rds, _ := cost.NewDimension(cost.DimensionService, "Amazon RDS")
	values := []cost.Cost{
		{Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Dimension: ec2, Amount: mustMoney(t, 30, "USD")},
		{Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Dimension: s3, Amount: mustMoney(t, 20, "USD")},
		{Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Dimension: rds, Amount: mustMoney(t, 10, "USD")},
	}
	limited, err := basecollector.LimitDimensions(values, 2, "__other__")
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 {
		t.Fatalf("limited len = %d, want 2", len(limited))
	}
	total := 0.0
	for _, value := range limited {
		total += value.Amount.Amount()
	}
	if total != 60 {
		t.Fatalf("total = %v, want 60", total)
	}
}

func TestLimitDimensionsRejectsMixedCurrencyAndReservedValue(t *testing.T) {
	service, _ := cost.NewDimension(cost.DimensionService, "Amazon EC2")
	other, _ := cost.NewDimension(cost.DimensionService, "__other__")
	values := []cost.Cost{
		{Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Dimension: service, Amount: mustMoney(t, 1, "USD")},
		{Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Dimension: service, Amount: mustMoney(t, 2, "EUR")},
	}
	if _, err := basecollector.LimitDimensions(values, 1, "__other__"); !errors.Is(err, basecollector.ErrMixedCurrency) {
		t.Fatalf("mixed currency error = %v", err)
	}
	if _, err := basecollector.LimitDimensions([]cost.Cost{
		{Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Dimension: other, Amount: mustMoney(t, 1, "USD")},
	}, 2, "__other__"); !errors.Is(err, basecollector.ErrReservedDimension) {
		t.Fatalf("reserved dimension error = %v", err)
	}
	if _, err := basecollector.LimitDimensions([]cost.Cost{
		{Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Dimension: other, Amount: mustMoney(t, 1, "USD")},
	}, 2, " __other__ "); !errors.Is(err, basecollector.ErrReservedDimension) {
		t.Fatalf("normalized reserved dimension error = %v", err)
	}
	if _, err := basecollector.LimitDimensions(nil, 0, "__other__"); !errors.Is(err, basecollector.ErrInvalidSeriesLimit) {
		t.Fatalf("invalid limit error = %v", err)
	}
	if _, err := basecollector.LimitDimensions(nil, 1, "   "); !errors.Is(err, basecollector.ErrInvalidOverflowLabel) {
		t.Fatalf("invalid overflow label error = %v", err)
	}
	if _, err := basecollector.LimitDimensions([]cost.Cost{{}}, 1, "__other__"); !errors.Is(err, basecollector.ErrInvalidCostIdentity) {
		t.Fatalf("invalid provider/basis error = %v", err)
	}
}

func mustMoney(t *testing.T, amount float64, currency string) cost.Money {
	t.Helper()
	value, err := cost.NewMoney(amount, currency)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
