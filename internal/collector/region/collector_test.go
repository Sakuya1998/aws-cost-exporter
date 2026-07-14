package region

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

var _ collector.Collector = (*Collector)(nil)

// TestCollectorNormalizesAndLimitsRegions verifies global normalization,
// cardinality limiting, overflow conservation, and Cost Explorer queries.
func TestCollectorNormalizesAndLimitsRegions(t *testing.T) {
	reader := &recordingReader{values: map[cost.Window][]regionValue{
		cost.WindowDaily: {
			{name: "", amount: 5}, {name: "us-east-1", amount: 40},
			{name: "eu-west-1", amount: 30}, {name: "ap-south-1", amount: 20},
		},
		cost.WindowMonthToDate: {{name: "", amount: 100}},
	}}
	subject, _ := New(reader, 3)
	snapshot, err := subject.Collect(
		context.Background(),
		time.Date(2026, 7, 13, 1, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)),
	)
	if subject.Name() != Name || len(reader.queries) != 2 {
		t.Fatalf("collector name/queries = %q/%d, want %q/2", subject.Name(), len(reader.queries), Name)
	}
	assertQuery(t, reader.queries[0], cost.WindowDaily, "2026-07-12", "2026-07-13")
	assertQuery(t, reader.queries[1], cost.WindowMonthToDate, "2026-07-01", "2026-07-13")
	costs := snapshot.Costs()
	if err != nil || len(costs) != 4 {
		t.Fatalf("Collect() returned error=%v costs=%#v, want four costs", err, costs)
	}
	assertCost(t, costs[0], cost.WindowDaily, OtherRegion, 25)
	assertCost(t, costs[1], cost.WindowDaily, "eu-west-1", 30)
	assertCost(t, costs[2], cost.WindowDaily, "us-east-1", 40)
	assertCost(t, costs[3], cost.WindowMonthToDate, "global", 100)
}

// TestCollectorBreaksAmountTiesByRegion verifies stable top-series selection.
func TestCollectorBreaksAmountTiesByRegion(t *testing.T) {
	reader := &recordingReader{values: map[cost.Window][]regionValue{
		cost.WindowDaily: {{name: "b", amount: 10}, {name: "a", amount: 10}, {name: "c", amount: 1}},
	}}
	subject, _ := New(reader, 2)
	snapshot, err := subject.Collect(context.Background(), time.Now())
	costs := snapshot.Costs()
	if err != nil || len(costs) != 2 {
		t.Fatalf("Collect() returned error=%v costs=%#v, want two costs", err, costs)
	}
	assertCost(t, costs[0], cost.WindowDaily, OtherRegion, 11)
	assertCost(t, costs[1], cost.WindowDaily, "a", 10)
}

// TestCollectorRejectsPartialOrMixedResults verifies unsafe refreshes publish
// no partial snapshot.
func TestCollectorRejectsPartialOrMixedResults(t *testing.T) {
	for _, window := range []cost.Window{cost.WindowDaily, cost.WindowMonthToDate} {
		reader := &recordingReader{failWindow: window}
		subject, _ := New(reader, 10)
		snapshot, err := subject.Collect(context.Background(), time.Now())
		if err == nil || len(snapshot.Costs()) != 0 {
			t.Fatalf("failure for %q returned error=%v costs=%#v", window, err, snapshot.Costs())
		}
	}
	reader := &recordingReader{values: map[cost.Window][]regionValue{
		cost.WindowDaily: {
			{name: "us-east-1", amount: 1}, {name: "eu-west-1", amount: 1, currency: "EUR"},
		},
	}}
	subject, _ := New(reader, 1)
	snapshot, err := subject.Collect(context.Background(), time.Now())
	if !errors.Is(err, ErrMixedCurrency) || len(snapshot.Costs()) != 0 {
		t.Fatalf("mixed currencies returned error=%v costs=%#v", err, snapshot.Costs())
	}
}

// TestNewRejectsInvalidDependencies verifies construction fails fast.
func TestNewRejectsInvalidDependencies(t *testing.T) {
	if subject, err := New(nil, 1); subject != nil || !errors.Is(err, ErrNilReader) {
		t.Fatalf("New(nil, 1) = %#v, %v; want ErrNilReader", subject, err)
	}
	if subject, err := New(&recordingReader{}, 0); subject != nil || !errors.Is(err, ErrInvalidSeriesLimit) {
		t.Fatalf("New(reader, 0) = %#v, %v; want ErrInvalidSeriesLimit", subject, err)
	}
}

type regionValue struct {
	name     string
	amount   float64
	currency string
}

type recordingReader struct {
	values     map[cost.Window][]regionValue
	queries    []ports.CostQuery
	failWindow cost.Window
}

func (reader *recordingReader) ReadCosts(_ context.Context, query ports.CostQuery) ([]cost.Cost, error) {
	reader.queries = append(reader.queries, query)
	if query.Window == reader.failWindow {
		return nil, errors.New("reader failure")
	}
	result := make([]cost.Cost, 0, len(reader.values[query.Window]))
	for _, value := range reader.values[query.Window] {
		currency := value.currency
		if currency == "" {
			currency = "USD"
		}
		money, _ := cost.NewMoney(value.amount, currency)
		dimension, _ := cost.NewDimension(cost.DimensionRegion, value.name)
		result = append(result, cost.Cost{
			Window: query.Window, Period: query.Period, Dimension: dimension, Amount: money,
		})
	}
	return result, nil
}

func assertQuery(t *testing.T, query ports.CostQuery, window cost.Window, start, end string) {
	t.Helper()
	if query.GroupBy != cost.DimensionRegion || query.Window != window ||
		query.Period.Start().Format(time.DateOnly) != start ||
		query.Period.End().Format(time.DateOnly) != end {
		t.Fatalf("query = %#v, want %q %s..%s region", query, window, start, end)
	}
}

func assertCost(t *testing.T, got cost.Cost, window cost.Window, region string, amount float64) {
	t.Helper()
	if got.Window != window || got.Dimension.Value() != region ||
		got.Amount.Amount() != amount || got.Amount.Currency() != "USD" {
		t.Fatalf("cost = %#v, want %q %s %.2f USD", got, window, region, amount)
	}
}
