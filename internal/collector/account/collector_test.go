package account

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

var _ collector.Collector = (*Collector)(nil)

// TestCollectorUsesAllowlistAndUTCWindows verifies isolated filters, query
// boundaries, dimensions, amounts, and currencies.
func TestCollectorUsesAllowlistAndUTCWindows(t *testing.T) {
	allowlist := []string{"444444444444", "333333333333", "222222222222", "111111111111"}
	reader := &recordingReader{values: map[cost.Window][]accountValue{
		cost.WindowDaily: {
			{id: "444444444444", amount: 40}, {id: "333333333333", amount: 30},
			{id: "222222222222", amount: 20}, {id: "111111111111", amount: 10},
		},
		cost.WindowMonthToDate: {{id: "111111111111", amount: 100}},
	}}
	subject, err := New(reader, allowlist, 3)
	if err != nil {
		t.Fatalf("New() returned an unexpected error: %v", err)
	}
	allowlist[0] = "999999999999"
	snapshot, err := subject.Collect(
		context.Background(),
		time.Date(2026, 7, 13, 1, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)),
	)
	if err != nil || subject.Name() != Name || len(reader.queries) != 2 {
		t.Fatalf("Collect() error/name/queries = %v/%q/%d", err, subject.Name(), len(reader.queries))
	}
	assertQuery(t, reader.queries[0], cost.WindowDaily, "2026-07-12", "2026-07-13")
	assertQuery(t, reader.queries[1], cost.WindowMonthToDate, "2026-07-01", "2026-07-13")
	costs := snapshot.Costs()
	if len(costs) != 4 || costs[0].Dimension.Value() != "333333333333" ||
		costs[2].Dimension.Value() != OtherAccount || costs[2].Amount.Amount() != 30 ||
		costs[0].Amount.Currency() != "USD" {
		t.Fatalf("Collect() costs = %#v, want bounded account costs", costs)
	}
}

// TestCollectorRejectsInvalidAccountIDs verifies fixed safe errors for both
// configuration and provider data.
func TestCollectorRejectsInvalidAccountIDs(t *testing.T) {
	if subject, err := New(&recordingReader{}, []string{"not-an-account"}, 1); subject != nil || !errors.Is(err, ErrInvalidAccountID) ||
		strings.Contains(err.Error(), "not-an-account") {
		t.Fatalf("New(invalid) = %#v, %v; want safe ErrInvalidAccountID", subject, err)
	}
	reader := &recordingReader{values: map[cost.Window][]accountValue{
		cost.WindowDaily: {{id: "private-invalid-id", amount: 1}},
	}}
	subject, _ := New(reader, nil, 10)
	snapshot, err := subject.Collect(context.Background(), time.Now())
	if !errors.Is(err, ErrInvalidAccountID) || strings.Contains(err.Error(), "private-invalid-id") ||
		len(snapshot.Costs()) != 0 {
		t.Fatalf("Collect() returned unsafe error=%v costs=%#v", err, snapshot.Costs())
	}
}

// TestCollectorRejectsPartialResults verifies either failed query prevents
// publishing any account costs.
func TestCollectorRejectsPartialResults(t *testing.T) {
	for _, window := range []cost.Window{cost.WindowDaily, cost.WindowMonthToDate} {
		reader := &recordingReader{failWindow: window}
		subject, _ := New(reader, nil, 10)
		snapshot, err := subject.Collect(context.Background(), time.Now())
		if err == nil || len(snapshot.Costs()) != 0 {
			t.Fatalf("failure for %q returned error=%v costs=%#v", window, err, snapshot.Costs())
		}
	}
}

// TestNewRejectsNilReader verifies invalid dependency injection fails fast.
func TestNewRejectsNilReader(t *testing.T) {
	if subject, err := New(nil, nil, 1); subject != nil || !errors.Is(err, ErrNilReader) {
		t.Fatalf("New(nil) = %#v, %v; want ErrNilReader", subject, err)
	}
	if subject, err := New(&recordingReader{}, nil, 0); subject != nil ||
		!errors.Is(err, ErrInvalidSeriesLimit) {
		t.Fatalf("New(reader, nil, 0) = %#v, %v; want ErrInvalidSeriesLimit", subject, err)
	}
}

type accountValue struct {
	id     string
	amount float64
}

type recordingReader struct {
	values     map[cost.Window][]accountValue
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
		money, _ := cost.NewMoney(value.amount, "USD")
		dimension, _ := cost.NewDimension(cost.DimensionAccount, value.id)
		result = append(result, cost.Cost{
			Window: query.Window, Period: query.Period, Dimension: dimension, Amount: money,
		})
	}
	return result, nil
}

func assertQuery(t *testing.T, query ports.CostQuery, window cost.Window, start, end string) {
	t.Helper()
	if query.GroupBy != cost.DimensionAccount || query.Window != window ||
		query.Period.Start().Format(time.DateOnly) != start ||
		query.Period.End().Format(time.DateOnly) != end ||
		len(query.LinkedAccountIDs) != 4 || query.LinkedAccountIDs[0] != "444444444444" {
		t.Fatalf("query = %#v, want filtered %q %s..%s account", query, window, start, end)
	}
	query.LinkedAccountIDs[0] = "reader-mutated-copy"
}
