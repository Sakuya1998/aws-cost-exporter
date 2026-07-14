package service

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

// TestCollectorLimitsServicesAndPreservesTotal verifies the series cap,
// deterministic amount ranking, overflow sum, and Cost Explorer queries.
func TestCollectorLimitsServicesAndPreservesTotal(t *testing.T) {
	reader := &recordingReader{values: map[cost.Window][]serviceValue{
		cost.WindowDaily: {
			{name: "D", amount: 20}, {name: "B", amount: 40},
			{name: "C", amount: 30}, {name: "A", amount: 10},
		},
		cost.WindowMonthToDate: {{name: "S", amount: 100}},
	}}
	observers := [2]overflowObserver{}
	subject, _ := New(reader, 3, &observers[0], &observers[1])
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
	assertCost(t, costs[0], cost.WindowDaily, "B", 40)
	assertCost(t, costs[1], cost.WindowDaily, "C", 30)
	assertCost(t, costs[2], cost.WindowDaily, OtherService, 30)
	assertCost(t, costs[3], cost.WindowMonthToDate, "S", 100)
	if observers[0] != (overflowObserver{dimension: "service", count: 2}) || observers[1] != observers[0] {
		t.Fatalf("overflow observations = %#v, want two service/2 records", observers)
	}
}

// TestCollectorBreaksAmountTiesByService verifies stable top-series selection.
func TestCollectorBreaksAmountTiesByService(t *testing.T) {
	reader := &recordingReader{values: map[cost.Window][]serviceValue{
		cost.WindowDaily: {{name: "B", amount: 10}, {name: "A", amount: 10}, {name: "C", amount: 1}},
	}}
	subject, _ := New(reader, 2)
	snapshot, err := subject.Collect(context.Background(), time.Now())
	costs := snapshot.Costs()
	if err != nil || len(costs) != 2 {
		t.Fatalf("Collect() returned error=%v costs=%#v, want two costs", err, costs)
	}
	assertCost(t, costs[0], cost.WindowDaily, "A", 10)
	assertCost(t, costs[1], cost.WindowDaily, OtherService, 11)
}

// TestCollectorRejectsPartialResults verifies failed queries publish nothing.
func TestCollectorRejectsPartialResults(t *testing.T) {
	for _, window := range []cost.Window{cost.WindowDaily, cost.WindowMonthToDate} {
		reader := &recordingReader{failWindow: window}
		subject, _ := New(reader, 10)
		snapshot, err := subject.Collect(context.Background(), time.Now())
		if err == nil || len(snapshot.Costs()) != 0 {
			t.Fatalf("failure for %q returned error=%v costs=%#v", window, err, snapshot.Costs())
		}
	}
	reader := &recordingReader{values: map[cost.Window][]serviceValue{
		cost.WindowDaily: {
			{name: "USD service", amount: 1}, {name: "EUR service", amount: 1, currency: "EUR"},
		},
	}}
	subject, _ := New(reader, 1)
	snapshot, err := subject.Collect(context.Background(), time.Now())
	if !errors.Is(err, ErrMixedCurrency) || len(snapshot.Costs()) != 0 {
		t.Fatalf("mixed currencies returned error=%v costs=%#v", err, snapshot.Costs())
	}
	reader = &recordingReader{values: map[cost.Window][]serviceValue{
		cost.WindowDaily: {{name: OtherService, amount: 1}},
	}}
	subject, _ = New(reader, 10)
	snapshot, err = subject.Collect(context.Background(), time.Now())
	if !errors.Is(err, collector.ErrReservedDimension) || len(snapshot.Costs()) != 0 {
		t.Fatalf("reserved service returned error=%v costs=%#v", err, snapshot.Costs())
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

type serviceValue struct {
	name     string
	amount   float64
	currency string
}

type overflowObserver struct {
	dimension string
	count     int
}

func (observer *overflowObserver) ObserveOverflow(dimension string, count int) {
	observer.dimension, observer.count = dimension, observer.count+count
}

type recordingReader struct {
	values     map[cost.Window][]serviceValue
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
		dimension, _ := cost.NewDimension(cost.DimensionService, value.name)
		result = append(result, cost.Cost{
			Window: query.Window, Period: query.Period, Dimension: dimension, Amount: money,
		})
	}
	return result, nil
}

func assertQuery(t *testing.T, query ports.CostQuery, window cost.Window, start, end string) {
	t.Helper()
	if query.GroupBy != cost.DimensionService || query.Window != window ||
		query.Period.Start().Format(time.DateOnly) != start ||
		query.Period.End().Format(time.DateOnly) != end {
		t.Fatalf("query = %#v, want %q %s..%s service", query, window, start, end)
	}
}

func assertCost(t *testing.T, got cost.Cost, window cost.Window, service string, amount float64) {
	t.Helper()
	if got.Window != window || got.Dimension.Value() != service ||
		got.Amount.Amount() != amount || got.Amount.Currency() != "USD" {
		t.Fatalf("cost = %#v, want %q %s %.2f USD", got, window, service, amount)
	}
}
