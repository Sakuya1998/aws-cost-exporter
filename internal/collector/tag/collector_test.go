package tag

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/tagcost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

type stubReader struct {
	rows []tagcost.Cost
	err  error
}

func (reader stubReader) ReadTagCosts(context.Context, ports.CostQuery, string) ([]tagcost.Cost, error) {
	return append([]tagcost.Cost(nil), reader.rows...), reader.err
}
func tagged(t *testing.T, name string, amount float64) tagcost.Cost {
	money, err := cost.NewMoney(amount, "USD")
	if err != nil {
		t.Fatal(err)
	}
	return tagcost.Cost{Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, TagKey: "Environment", TagValue: name, Amount: money}
}

func TestCollectorBoundsEachTagQueryAndConservesAmount(t *testing.T) {
	reader := stubReader{rows: []tagcost.Cost{tagged(t, "prod", 5), tagged(t, "dev", 3), tagged(t, "test", 2)}}
	subject, err := New("payer", reader, []cost.Basis{cost.BasisUnblended}, []config.TagKeyConfig{{Key: "Environment", MaxValues: 2}}, 4, "__other__")
	if err != nil {
		t.Fatal(err)
	}
	result, err := subject.Collect(context.Background(), time.Now())
	if err != nil || len(result.TagCosts()) != 4 {
		t.Fatalf("tags=%#v err=%v", result.TagCosts(), err)
	}
	total := 0.0
	for _, value := range result.TagCosts() {
		total += value.Amount.Amount()
		if value.Target != "payer" {
			t.Fatal("target missing")
		}
	}
	if total != 20 {
		t.Fatalf("total=%v", total)
	}
	failed, _ := New("payer", stubReader{err: errors.New("failed")}, []cost.Basis{cost.BasisUnblended}, []config.TagKeyConfig{{Key: "Environment", MaxValues: 2}}, 4, "__other__")
	if _, err := failed.Collect(context.Background(), time.Now()); err == nil {
		t.Fatal("ignored reader error")
	}
	if value, err := New("payer", nil, nil, nil, 0, ""); value != nil || err == nil {
		t.Fatal("accepted invalid config")
	}
}

func TestLimitValuesRejectsCrossBoundaryOverflow(t *testing.T) {
	values := []tagcost.Cost{tagged(t, "prod", 5), tagged(t, "dev", 3), tagged(t, "test", 2)}
	values[2].Provider = cost.ProviderCURAthena
	if _, err := LimitValues(values, 2, "__other__"); err == nil {
		t.Fatal("aggregated tag values across providers")
	}
	values[2].Provider = cost.ProviderCostExplorer
	other, _ := cost.NewMoney(2, "EUR")
	values[2].Amount = other
	if _, err := LimitValues(values, 2, "__other__"); err == nil {
		t.Fatal("aggregated tag values across currencies")
	}
}
