package memory

import (
	"sync"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

var _ ports.SnapshotStore = (*Store)(nil)

// TestStorePublishesAtomicSnapshots verifies merged snapshots and status views.
func TestStorePublishesAtomicSnapshots(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)}
	store, _ := New(clock, time.Hour, 2*time.Hour)
	_ = store.Publish("total", partialCost(1, cost.DimensionTotal, ""))
	view := store.Load()
	status := view.Collectors["total"]
	if len(view.Snapshot.Costs()) != 1 || !status.Up ||
		status.Freshness != ports.FreshnessFresh || status.LastSuccess != clock.now || status.Series != 1 {
		t.Fatalf("Load() = %#v, want one fresh successful total", view)
	}
	clock.Advance(90 * time.Minute)
	if got := store.Load().Collectors["total"].Freshness; got != ports.FreshnessAging {
		t.Fatalf("freshness = %q, want %q", got, ports.FreshnessAging)
	}
	_ = store.Publish("service", partialCost(2, cost.DimensionService, "EC2"))
	view = store.Load()
	if len(view.Snapshot.Costs()) != 2 {
		t.Fatalf("merged snapshot contains %d costs, want 2", len(view.Snapshot.Costs()))
	}
	delete(view.Collectors, "total")
	if _, exists := store.Load().Collectors["total"]; !exists {
		t.Fatal("mutating a loaded status map changed cache state")
	}
	clock.Advance(2 * time.Hour)
	if got := store.Load().Collectors["total"].Freshness; got != ports.FreshnessStale {
		t.Fatalf("freshness = %q, want %q", got, ports.FreshnessStale)
	}
}

// TestStoreFailureRetainsLastSuccess verifies failures retain published data.
func TestStoreFailureRetainsLastSuccess(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)}
	store, _ := New(clock, time.Hour, 2*time.Hour)
	_ = store.Publish("total", partialCost(1, cost.DimensionTotal, ""))
	successTime := clock.now
	clock.Advance(time.Minute)
	if err := store.RecordFailure("total"); err != nil {
		t.Fatalf("RecordFailure() returned an unexpected error: %v", err)
	}
	view := store.Load()
	status := view.Collectors["total"]
	if len(view.Snapshot.Costs()) != 1 || status.Up ||
		status.LastSuccess != successTime || status.LastAttempt != clock.now {
		t.Fatalf("Load() after failure = %#v, want retained failed total", view)
	}
	_ = store.Publish("total", partialCost(3, cost.DimensionTotal, ""))
	costs := store.Snapshot().Costs()
	if len(costs) != 1 || costs[0].Amount.Amount() != 3 {
		t.Fatalf("replacement costs = %#v, want only latest value", costs)
	}
	_ = store.RecordFailure("forecast")
	if store.Load().Collectors["forecast"].Freshness != ports.FreshnessMissing {
		t.Fatal("collector without a success must be missing")
	}
}

// TestStoreConcurrentReadersNeverObservePartialPublish exercises lock-free reads.
func TestStoreConcurrentReadersNeverObservePartialPublish(t *testing.T) {
	store, _ := New(&fakeClock{now: time.Now()}, time.Hour, 2*time.Hour)
	var wait sync.WaitGroup
	wait.Add(3)
	for _, name := range []string{"service", "region"} {
		go func() {
			defer wait.Done()
			for index := 0; index < 500; index++ {
				_ = store.Publish(name, twoServiceCosts(float64(index)))
			}
		}()
	}
	go func() {
		defer wait.Done()
		for index := 0; index < 500; index++ {
			length := len(store.Snapshot().Costs())
			if length != 0 && length != 2 && length != 4 {
				t.Errorf("observed partial snapshot length %d", length)
				return
			}
		}
	}()
	wait.Wait()
	if length := len(store.Snapshot().Costs()); length != 4 {
		t.Fatalf("concurrent writers retained %d costs, want 4", length)
	}
}

// TestNewAndCollectorNamesRejectInvalidInput verifies safe fail-fast behavior.
func TestNewAndCollectorNamesRejectInvalidInput(t *testing.T) {
	if store, err := New(nil, time.Hour, 2*time.Hour); store != nil || err == nil {
		t.Fatalf("New(nil) = %#v, %v; want error", store, err)
	}
	store, _ := New(&fakeClock{}, time.Hour, 2*time.Hour)
	if err := store.Publish("", cost.PartialSnapshot{}); err == nil {
		t.Fatal("Publish() accepted an empty collector name")
	}
	if err := store.RecordFailure(" "); err == nil {
		t.Fatal("RecordFailure() accepted an empty collector name")
	}
}

type fakeClock struct {
	now time.Time
}

func (clock *fakeClock) Now() time.Time { return clock.now }

func (clock *fakeClock) Advance(duration time.Duration) { clock.now = clock.now.Add(duration) }

func partialCost(amount float64, kind cost.DimensionKind, value string) cost.PartialSnapshot {
	money, _ := cost.NewMoney(amount, "USD")
	dimension, _ := cost.NewDimension(kind, value)
	return cost.NewSnapshot([]cost.Cost{{
		Window: cost.WindowDaily, Period: cost.DayContaining(time.Now()),
		Dimension: dimension, Amount: money,
	}}, nil)
}

func twoServiceCosts(amount float64) cost.PartialSnapshot {
	left := partialCost(amount, cost.DimensionService, "A")
	right := partialCost(amount, cost.DimensionService, "B")
	return cost.MergeSnapshots(left, right)
}
