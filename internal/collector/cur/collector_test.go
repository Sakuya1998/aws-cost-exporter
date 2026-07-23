package cur

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/tagcost"
)

type stubReader struct {
	costs           []cost.Cost
	tags            []tagcost.Cost
	costErr, tagErr error
	tagCalls        *int
}

func (reader stubReader) QueryCosts(context.Context, time.Time, []cost.Basis) ([]cost.Cost, error) {
	return reader.costs, reader.costErr
}
func (reader stubReader) QueryTagCosts(context.Context, time.Time, []cost.Basis) ([]tagcost.Cost, error) {
	if reader.tagCalls != nil {
		*reader.tagCalls++
	}
	return reader.tags, reader.tagErr
}

func TestCollectorPublishesCURAndRejectsUnsafeTags(t *testing.T) {
	money, _ := cost.NewMoney(1, "USD")
	dimension, _ := cost.NewDimension(cost.DimensionTotal, "")
	reader := stubReader{costs: []cost.Cost{{Target: "payer", Provider: cost.ProviderCURAthena, Basis: cost.BasisNet, Window: cost.WindowDaily, Dimension: dimension, Amount: money}}, tags: []tagcost.Cost{{Target: "payer", Provider: cost.ProviderCURAthena, Basis: cost.BasisNet, Window: cost.WindowDaily, TagKey: "Environment", TagValue: "prod", Amount: money}}}
	subject, err := New("payer", reader, []cost.Basis{cost.BasisNet}, true, 10, 5, 1, []config.TagKeyConfig{{Key: "Environment", MaxValues: 2}}, "__other__")
	if err != nil {
		t.Fatal(err)
	}
	result, err := subject.Collect(context.Background(), time.Now())
	if err != nil || len(result.Costs()) != 1 || len(result.TagCosts()) != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	reader.tags[0].TagKey = "Secret"
	unsafe, _ := New("payer", reader, []cost.Basis{cost.BasisNet}, true, 10, 5, 1, []config.TagKeyConfig{{Key: "Environment", MaxValues: 2}}, "__other__")
	if _, err := unsafe.Collect(context.Background(), time.Now()); err == nil {
		t.Fatal("accepted non-allowlisted tag")
	}
	reader.costErr = errors.New("failed")
	failed, _ := New("payer", reader, nil, false, 10, 5, 1, nil, "__other__")
	if _, err := failed.Collect(context.Background(), time.Now()); err == nil {
		t.Fatal("ignored cost error")
	}
	if value, err := New("payer", nil, nil, false, 0, 0, 0, nil, ""); value != nil || err == nil {
		t.Fatal("accepted invalid config")
	}
}

func TestCollectorRejectsCurrenciesBeyondConfiguredLimit(t *testing.T) {
	usd, _ := cost.NewMoney(1, "USD")
	eur, _ := cost.NewMoney(1, "EUR")
	dimension, _ := cost.NewDimension(cost.DimensionTotal, "")
	reader := stubReader{
		costs: []cost.Cost{{Target: "payer", Provider: cost.ProviderCURAthena, Basis: cost.BasisNet, Window: cost.WindowDaily, Dimension: dimension, Amount: usd}},
		tags:  []tagcost.Cost{{Target: "payer", Provider: cost.ProviderCURAthena, Basis: cost.BasisNet, Window: cost.WindowDaily, TagKey: "Environment", TagValue: "prod", Amount: eur}},
	}

	subject, err := New("payer", reader, []cost.Basis{cost.BasisNet}, true, 10, 5, 1, []config.TagKeyConfig{{Key: "Environment", MaxValues: 2}}, "__other__")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := subject.Collect(context.Background(), time.Now()); err == nil || err.Error() != "CUR currency limit exceeded" {
		t.Fatalf("Collect() error=%v", err)
	}

	allowed, err := New("payer", reader, []cost.Basis{cost.BasisNet}, true, 10, 5, 2, []config.TagKeyConfig{{Key: "Environment", MaxValues: 2}}, "__other__")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := allowed.Collect(context.Background(), time.Now()); err != nil {
		t.Fatalf("Collect() with two currencies: %v", err)
	}
}

func TestCollectorSkipsTagQueryWhenCostCurrenciesExceedLimit(t *testing.T) {
	usd, _ := cost.NewMoney(1, "USD")
	eur, _ := cost.NewMoney(1, "EUR")
	dimension, _ := cost.NewDimension(cost.DimensionTotal, "")
	tagCalls := 0
	reader := stubReader{
		costs: []cost.Cost{
			{Target: "payer", Provider: cost.ProviderCURAthena, Basis: cost.BasisNet, Window: cost.WindowDaily, Dimension: dimension, Amount: usd},
			{Target: "payer", Provider: cost.ProviderCURAthena, Basis: cost.BasisNet, Window: cost.WindowDaily, Dimension: dimension, Amount: eur},
		},
		tagCalls: &tagCalls,
	}
	subject, err := New("payer", reader, []cost.Basis{cost.BasisNet}, true, 10, 5, 1, []config.TagKeyConfig{{Key: "Environment", MaxValues: 2}}, "__other__")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := subject.Collect(context.Background(), time.Now()); err == nil || err.Error() != "CUR currency limit exceeded" {
		t.Fatalf("Collect() error=%v", err)
	}
	if tagCalls != 0 {
		t.Fatalf("QueryTagCosts calls=%d want 0", tagCalls)
	}
}
