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
}

func (reader stubReader) ReadCosts(context.Context, time.Time, []cost.Basis) ([]cost.Cost, error) {
	return reader.costs, reader.costErr
}
func (reader stubReader) ReadTagCosts(context.Context, time.Time, []cost.Basis) ([]tagcost.Cost, error) {
	return reader.tags, reader.tagErr
}

func TestCollectorPublishesCURAndRejectsUnsafeTags(t *testing.T) {
	money, _ := cost.NewMoney(1, "USD")
	dimension, _ := cost.NewDimension(cost.DimensionTotal, "")
	reader := stubReader{costs: []cost.Cost{{Target: "payer", Provider: cost.ProviderCURAthena, Basis: cost.BasisNet, Window: cost.WindowDaily, Dimension: dimension, Amount: money}}, tags: []tagcost.Cost{{Target: "payer", Provider: cost.ProviderCURAthena, Basis: cost.BasisNet, Window: cost.WindowDaily, TagKey: "Environment", TagValue: "prod", Amount: money}}}
	subject, err := New("payer", reader, []cost.Basis{cost.BasisNet}, true, 10, 5, []config.TagKeyConfig{{Key: "Environment", MaxValues: 2}}, "__other__")
	if err != nil {
		t.Fatal(err)
	}
	result, err := subject.Collect(context.Background(), time.Now())
	if err != nil || len(result.Costs()) != 1 || len(result.TagCosts()) != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	reader.tags[0].TagKey = "Secret"
	unsafe, _ := New("payer", reader, []cost.Basis{cost.BasisNet}, true, 10, 5, []config.TagKeyConfig{{Key: "Environment", MaxValues: 2}}, "__other__")
	if _, err := unsafe.Collect(context.Background(), time.Now()); err == nil {
		t.Fatal("accepted non-allowlisted tag")
	}
	reader.costErr = errors.New("failed")
	failed, _ := New("payer", reader, nil, false, 10, 5, nil, "__other__")
	if _, err := failed.Collect(context.Background(), time.Now()); err == nil {
		t.Fatal("ignored cost error")
	}
	if value, err := New("payer", nil, nil, false, 0, 0, nil, ""); value != nil || err == nil {
		t.Fatal("accepted invalid config")
	}
}
