package total

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

// TestCollectorBuildsUTCWindows verifies the stable collector contract and
// the exclusive daily and month-to-date query boundaries.
func TestCollectorBuildsUTCWindows(t *testing.T) {
	t.Parallel()
	reader := &recordingReader{}
	subject, err := New(reader)
	if err != nil {
		t.Fatalf("New() returned an unexpected error: %v", err)
	}

	reference := time.Date(2026, 7, 13, 1, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	snapshot, err := subject.Collect(context.Background(), reference)
	if err != nil {
		t.Fatalf("Collect() returned an unexpected error: %v", err)
	}
	if subject.Name() != Name || len(reader.queries) != 2 {
		t.Fatalf("collector name/queries = %q/%d, want %q/2", subject.Name(), len(reader.queries), Name)
	}
	assertQuery(t, reader.queries[0], cost.WindowDaily, "2026-07-12", "2026-07-13")
	assertQuery(t, reader.queries[1], cost.WindowMonthToDate, "2026-07-01", "2026-07-13")

	costs := snapshot.Costs()
	if len(costs) != 2 || costs[0].Amount.Amount() != 1 ||
		costs[1].Amount.Amount() != 10 || costs[0].Amount.Currency() != "USD" {
		t.Fatalf("Collect() costs = %#v, want daily 1 USD and MTD 10 USD", costs)
	}
}

// TestCollectorLeavesMissingDailyCostAbsent verifies empty AWS data is not
// converted into a misleading zero-valued observation.
func TestCollectorLeavesMissingDailyCostAbsent(t *testing.T) {
	t.Parallel()
	subject, _ := New(&recordingReader{emptyDaily: true})
	snapshot, err := subject.Collect(context.Background(), time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Collect() returned an unexpected error: %v", err)
	}
	costs := snapshot.Costs()
	if len(costs) != 1 || costs[0].Window != cost.WindowMonthToDate {
		t.Fatalf("Collect() costs = %#v, want only month-to-date cost", costs)
	}
}

// TestCollectorRejectsPartialResults verifies either failed query prevents
// publishing any part of the refresh.
func TestCollectorRejectsPartialResults(t *testing.T) {
	t.Parallel()
	for _, window := range []cost.Window{cost.WindowDaily, cost.WindowMonthToDate} {
		reader := &recordingReader{failWindow: window}
		subject, _ := New(reader)
		snapshot, err := subject.Collect(context.Background(), time.Now())
		if err == nil || len(snapshot.Costs()) != 0 {
			t.Fatalf("failure for %q returned error=%v costs=%#v", window, err, snapshot.Costs())
		}
	}
}

// TestNewRejectsNilReader verifies invalid dependency injection fails fast.
func TestNewRejectsNilReader(t *testing.T) {
	t.Parallel()
	if subject, err := New(nil); subject != nil || !errors.Is(err, ErrNilReader) {
		t.Fatalf("New(nil) = %#v, %v; want nil and ErrNilReader", subject, err)
	}
}

type recordingReader struct {
	queries    []ports.CostQuery
	emptyDaily bool
	failWindow cost.Window
}

func (reader *recordingReader) ReadCosts(_ context.Context, query ports.CostQuery) ([]cost.Cost, error) {
	reader.queries = append(reader.queries, query)
	if query.Window == reader.failWindow {
		return nil, errors.New("reader failure")
	}
	if query.Window == cost.WindowDaily && reader.emptyDaily {
		return nil, nil
	}
	amount := 1.0
	if query.Window == cost.WindowMonthToDate {
		amount = 10
	}
	money, _ := cost.NewMoney(amount, "USD")
	dimension, _ := cost.NewDimension(cost.DimensionTotal, "")
	return []cost.Cost{{
		Window: query.Window, Period: query.Period, Dimension: dimension, Amount: money,
	}}, nil
}

func assertQuery(t *testing.T, query ports.CostQuery, window cost.Window, start, end string) {
	t.Helper()
	wantStart, _ := time.Parse(time.DateOnly, start)
	wantEnd, _ := time.Parse(time.DateOnly, end)
	if query.Window != window || query.GroupBy != cost.DimensionTotal ||
		!query.Period.Start().Equal(wantStart) || !query.Period.End().Equal(wantEnd) {
		t.Fatalf("query = %#v, want %q %s..%s total", query, window, start, end)
	}
}
