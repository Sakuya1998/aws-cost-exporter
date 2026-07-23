package memory

import (
	"sync"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/organization"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

var _ ports.SnapshotStore = (*Store)(nil)

func collectorID(target, name string) identity.CollectorID {
	return identity.CollectorID{Target: identity.TargetID(target), Name: name}
}

func TestStorePublishesTargetScopedAtomicSnapshots(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)}
	store, _ := New(clock, time.Hour, 2*time.Hour)
	a, b := collectorID("target-a", "total"), collectorID("target-b", "total")
	if err := store.Publish(a, partialCost("target-a", 1, cost.DimensionTotal, "")); err != nil {
		t.Fatal(err)
	}
	if err := store.Publish(b, partialCost("target-b", 2, cost.DimensionTotal, "")); err != nil {
		t.Fatal(err)
	}
	view := store.Load()
	if len(view.Snapshot.Costs()) != 2 || !view.Collectors[a].Up || !view.Collectors[b].Up {
		t.Fatalf("Load()=%#v", view)
	}
	clock.Advance(90 * time.Minute)
	if view = store.Load(); view.Collectors[a].Freshness != ports.FreshnessAging {
		t.Fatalf("freshness=%q", view.Collectors[a].Freshness)
	}
	delete(view.Collectors, a)
	if _, ok := store.Load().Collectors[a]; !ok {
		t.Fatal("loaded status map mutated cache")
	}
	clock.Advance(2 * time.Hour)
	if store.Load().Collectors[a].Freshness != ports.FreshnessStale {
		t.Fatal("expected stale target status")
	}
}

func TestStoreFailureRetainsOnlyAffectedTargetData(t *testing.T) {
	clock := &fakeClock{now: time.Now().UTC()}
	store, _ := New(clock, time.Hour, 2*time.Hour)
	a, b := collectorID("a", "total"), collectorID("b", "total")
	_ = store.Publish(a, partialCost("a", 1, cost.DimensionTotal, ""))
	_ = store.Publish(b, partialCost("b", 2, cost.DimensionTotal, ""))
	clock.Advance(time.Minute)
	if err := store.RecordFailure(a); err != nil {
		t.Fatal(err)
	}
	view := store.Load()
	if view.Collectors[a].Up || !view.Collectors[b].Up || len(view.Snapshot.Costs()) != 2 {
		t.Fatalf("isolated failure=%#v", view)
	}
	_ = store.Publish(a, partialCost("a", 3, cost.DimensionTotal, ""))
	values := store.Snapshot().Costs()
	if len(values) != 2 || values[0].Amount.Amount() != 3 {
		t.Fatalf("replacement=%#v", values)
	}
}

func TestStoreFiltersOrganizationsByObservedOrAllowlist(t *testing.T) {
	policies := map[identity.TargetID]OrganizationPolicy{
		"observed":    {SeriesLimit: 2},
		"allowlisted": {AccountIDs: []string{"333333333333"}, SeriesLimit: 1},
	}
	store, _ := New(&fakeClock{now: time.Now()}, time.Hour, 2*time.Hour, WithOrganizationPolicies(policies))
	accounts := []organization.Account{
		{Target: "observed", AccountID: "111111111111", Name: "one", Status: "ACTIVE"}, {Target: "observed", AccountID: "222222222222", Name: "two", Status: "ACTIVE"},
		{Target: "allowlisted", AccountID: "333333333333", Name: "three", Status: "ACTIVE"}, {Target: "allowlisted", AccountID: "444444444444", Name: "four", Status: "ACTIVE"},
	}
	_ = store.Publish(collectorID("observed", "organizations"), snapshot.New(nil, nil, nil, accounts[:2]))
	_ = store.Publish(collectorID("allowlisted", "organizations"), snapshot.New(nil, nil, nil, accounts[2:]))
	if got := len(store.Snapshot().Accounts()); got != 1 {
		t.Fatalf("before observed cost accounts=%d,want allowlist only", got)
	}
	if got := store.Load().Collectors[collectorID("observed", "organizations")].Series; got != 0 {
		t.Fatalf("unexported organization series=%d", got)
	}
	_ = store.Publish(collectorID("observed", "account"), partialCost("observed", 1, cost.DimensionAccount, "111111111111"))
	values := store.Snapshot().Accounts()
	if len(values) != 2 || values[0].AccountID != "333333333333" && values[1].AccountID != "333333333333" {
		t.Fatalf("filtered accounts=%#v", values)
	}
	if got := store.Load().Collectors[collectorID("observed", "organizations")].Series; got != 1 {
		t.Fatalf("selected organization series=%d", got)
	}
}

func TestStoreConcurrentReadersNeverObservePartialPublish(t *testing.T) {
	store, _ := New(&fakeClock{now: time.Now()}, time.Hour, 2*time.Hour)
	var wait sync.WaitGroup
	wait.Add(3)
	for _, target := range []string{"a", "b"} {
		target := target
		go func() {
			defer wait.Done()
			for i := 0; i < 300; i++ {
				_ = store.Publish(collectorID(target, "service"), twoServiceCosts(target, float64(i)))
			}
		}()
	}
	go func() {
		defer wait.Done()
		for i := 0; i < 300; i++ {
			length := len(store.Snapshot().Costs())
			if length != 0 && length != 2 && length != 4 {
				t.Errorf("partial length %d", length)
				return
			}
		}
	}()
	wait.Wait()
	if len(store.Snapshot().Costs()) != 4 {
		t.Fatal("missing concurrent target values")
	}
}

func TestStoreRejectsInvalidIdentity(t *testing.T) {
	if store, err := New(nil, time.Hour, 2*time.Hour); store != nil || err == nil {
		t.Fatal("New accepted nil clock")
	}
	store, _ := New(&fakeClock{}, time.Hour, 2*time.Hour)
	if err := store.Publish(identity.CollectorID{}, snapshot.PartialSnapshot{}); err == nil {
		t.Fatal("Publish accepted empty identity")
	}
	if err := store.RecordFailure(identity.CollectorID{}); err == nil {
		t.Fatal("RecordFailure accepted empty identity")
	}
	if err := store.Publish(collectorID("a", "total"), partialCost("b", 1, cost.DimensionTotal, "")); err == nil {
		t.Fatal("Publish accepted cross-target data")
	}
	duplicate := snapshot.Merge(partialCost("a", 1, cost.DimensionTotal, ""), partialCost("a", 2, cost.DimensionTotal, ""))
	if err := store.Publish(collectorID("a", "total"), duplicate); err == nil {
		t.Fatal("Publish accepted duplicate metric labels")
	}
}

type fakeClock struct{ now time.Time }

func (clock *fakeClock) Now() time.Time              { return clock.now }
func (clock *fakeClock) Advance(value time.Duration) { clock.now = clock.now.Add(value) }

func partialCost(target string, amount float64, kind cost.DimensionKind, value string) snapshot.PartialSnapshot {
	money, _ := cost.NewMoney(amount, "USD")
	dimension, _ := cost.NewDimension(kind, value)
	return snapshot.New([]cost.Cost{{Target: identity.TargetID(target), Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: cost.WindowDaily, Period: cost.DayContaining(time.Now()), Dimension: dimension, Amount: money}}, nil, nil, nil)
}
func twoServiceCosts(target string, amount float64) snapshot.PartialSnapshot {
	return snapshot.Merge(partialCost(target, amount, cost.DimensionService, "A"), partialCost(target, amount, cost.DimensionService, "B"))
}
