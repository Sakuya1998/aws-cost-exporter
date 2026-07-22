package collector

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

func TestValidateOverflowLabelRejectsEmpty(t *testing.T) {
	if err := ValidateOverflowLabel(""); !errors.Is(err, ErrInvalidOverflowLabel) {
		t.Fatalf("ValidateOverflowLabel(\"\") = %v, want ErrInvalidOverflowLabel", err)
	}
	if err := ValidateOverflowLabel(DefaultOverflowLabel); err != nil {
		t.Fatalf("ValidateOverflowLabel(default) = %v", err)
	}
}

type groupedReader struct {
	err  error
	seen []ports.CostQuery
}

func (reader *groupedReader) ReadCosts(_ context.Context, query ports.CostQuery) ([]cost.Cost, error) {
	reader.seen = append(reader.seen, query)
	if reader.err != nil {
		return nil, reader.err
	}
	service, _ := cost.NewDimension(cost.DimensionService, "Amazon EC2")
	s3, _ := cost.NewDimension(cost.DimensionService, "Amazon S3")
	rds, _ := cost.NewDimension(cost.DimensionService, "Amazon RDS")
	return []cost.Cost{
		{Dimension: service, Amount: moneyForGroupedTest(5)},
		{Dimension: s3, Amount: moneyForGroupedTest(3)},
		{Dimension: rds, Amount: moneyForGroupedTest(2)},
	}, nil
}

func moneyForGroupedTest(amount float64) cost.Money {
	value, _ := cost.NewMoney(amount, "USD")
	return value
}

func TestCollectGroupedBuildsBothWindowsAndPublishesBoundedTargetData(t *testing.T) {
	reader := &groupedReader{}
	mutations := 0
	validations := 0
	result, err := CollectGrouped(context.Background(), time.Now(), identity.TargetID("payer"), cost.DimensionService, 2, "__other__", reader,
		func(query *ports.CostQuery) { mutations++; query.Services = []string{"Amazon EC2"} },
		func(values []cost.Cost) error { validations++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if mutations != 2 || validations != 2 || len(reader.seen) != 2 || len(result.Costs()) != 4 {
		t.Fatalf("mutations=%d validations=%d queries=%d costs=%d", mutations, validations, len(reader.seen), len(result.Costs()))
	}
	for _, value := range result.Costs() {
		if value.Target != "payer" || value.Dimension.Value() == "" {
			t.Fatalf("invalid published value=%#v", value)
		}
	}
}

func TestCollectGroupedRejectsCancellationReaderAndValidationErrors(t *testing.T) {
	reference := time.Now()
	if _, err := CollectGrouped(context.Background(), reference, "payer", cost.DimensionService, 2, "   ", &groupedReader{}, nil, nil); !errors.Is(err, ErrInvalidOverflowLabel) {
		t.Fatalf("overflow error=%v", err)
	}
	reader := &groupedReader{err: errors.New("reader failed")}
	if _, err := CollectGrouped(context.Background(), reference, "payer", cost.DimensionService, 2, "__other__", reader, nil, nil); err == nil {
		t.Fatal("ignored reader error")
	}
	reader = &groupedReader{}
	if _, err := CollectGrouped(context.Background(), reference, "payer", cost.DimensionService, 2, "__other__", reader, nil, func([]cost.Cost) error { return errors.New("invalid values") }); err == nil {
		t.Fatal("ignored validation error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CollectGrouped(ctx, reference, "payer", cost.DimensionService, 2, "__other__", &groupedReader{}, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error=%v", err)
	}
}
