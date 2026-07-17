package scheduler

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
)

func TestRunnerRefreshesIndependentJobs(t *testing.T) {
	clock := newFakeClock()
	store := newFakeStore()
	calls := make(chan identity.CollectorID, 4)
	a := newCollector("target-a", "total", func(context.Context, time.Time) (snapshot.PartialSnapshot, error) {
		calls <- id("target-a", "total")
		return snapshot.PartialSnapshot{}, nil
	})
	b := newCollector("target-b", "budgets", func(context.Context, time.Time) (snapshot.PartialSnapshot, error) {
		calls <- id("target-b", "budgets")
		return snapshot.PartialSnapshot{}, nil
	})
	runner, err := NewJobs([]Job{{Collector: a, Interval: 10 * time.Minute, StartupRefresh: true}, {Collector: b, Interval: time.Hour, StartupRefresh: false}}, store, clock, func() float64 { return .5 }, validSchedulerConfig())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { runner.Run(ctx); close(done) }()
	if got := receive(t, calls); got != a.ID() {
		t.Fatalf("startup=%v", got)
	}
	receive(t, store.published)
	timers := []fakeTimer{receive(t, clock.timers), receive(t, clock.timers)}
	delays := map[time.Duration]fakeTimer{}
	for _, timer := range timers {
		delays[timer.delay] = timer
	}
	if delays[10*time.Minute+30*time.Second].fire == nil || delays[time.Hour+3*time.Minute].fire == nil {
		t.Fatalf("job delays=%v", delays)
	}
	delays[time.Hour+3*time.Minute].fire <- clock.Now()
	if got := receive(t, calls); got != b.ID() {
		t.Fatalf("periodic=%v", got)
	}
	receive(t, store.published)
	cancel()
	receive(t, done)
}

func TestRunnerEnforcesGlobalConcurrencyAndTargetSingleFlight(t *testing.T) {
	clock := newFakeClock()
	started, release := make(chan identity.CollectorID, 2), make(chan struct{}, 2)
	var mu sync.Mutex
	active, maximum := 0, 0
	makeOne := func(target string) *fakeCollector {
		return newCollector(target, "total", func(context.Context, time.Time) (snapshot.PartialSnapshot, error) {
			mu.Lock()
			active++
			if active > maximum {
				maximum = active
			}
			mu.Unlock()
			started <- id(target, "total")
			<-release
			mu.Lock()
			active--
			mu.Unlock()
			return snapshot.PartialSnapshot{}, nil
		})
	}
	observer := newFakeObserver()
	store := newFakeStore()
	runner, _ := NewJobs([]Job{{Collector: makeOne("a"), Interval: time.Hour, StartupRefresh: true}, {Collector: makeOne("b"), Interval: time.Hour, StartupRefresh: true}}, store, clock, nil, Config{JitterRatio: 0, MaxConcurrency: 1, Backoff: BackoffConfig{Initial: time.Minute, Max: time.Minute, Multiplier: 2}, Observer: observer})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { runner.Run(ctx); close(done) }()
	receive(t, started)
	timerA, timerB := receive(t, clock.timers), receive(t, clock.timers)
	timerA.fire <- clock.Now()
	timerB.fire <- clock.Now()
	for range 2 {
		if event := receive(t, observer.skipped); event.reason != "single_flight" {
			t.Fatalf("skip=%#v", event)
		}
	}
	release <- struct{}{}
	receive(t, started)
	release <- struct{}{}
	receive(t, store.published)
	receive(t, store.published)
	cancel()
	receive(t, done)
	mu.Lock()
	defer mu.Unlock()
	if maximum != 1 {
		t.Fatalf("maximum concurrency=%d", maximum)
	}
}

func TestRunnerFailureIsolationLoggingAndCancellation(t *testing.T) {
	clock := newFakeClock()
	store := newFakeStore()
	observer := newFakeObserver()
	var logs bytes.Buffer
	failing := newCollector("broken-target", "forecast", func(context.Context, time.Time) (snapshot.PartialSnapshot, error) {
		return snapshot.PartialSnapshot{}, errors.New("private unavailable detail")
	})
	healthy := newCollector("healthy-target", "total", func(context.Context, time.Time) (snapshot.PartialSnapshot, error) {
		return snapshot.PartialSnapshot{}, nil
	})
	runner, _ := NewJobs([]Job{{Collector: failing, Interval: time.Hour, StartupRefresh: true}, {Collector: healthy, Interval: time.Hour, StartupRefresh: true}}, store, clock, nil, Config{MaxConcurrency: 2, Backoff: BackoffConfig{Initial: time.Minute, Max: time.Minute, Multiplier: 2}, Observer: observer, Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { runner.Run(ctx); close(done) }()
	failed := receive(t, store.failed)
	published := receive(t, store.published)
	if failed != failing.ID() || published != healthy.ID() {
		t.Fatalf("failed=%v published=%v", failed, published)
	}
	text := logs.String()
	if !strings.Contains(text, `"target":"broken-target"`) || !strings.Contains(text, `"collector":"forecast"`) || strings.Contains(text, "private unavailable") {
		t.Fatalf("unsafe log=%s", text)
	}
	cancel()
	receive(t, done)
}

func TestRunnerStopsCanceledCollectorAndClearsState(t *testing.T) {
	started := make(chan struct{})
	observer := newFakeObserver()
	collector := newCollector("a", "total", func(ctx context.Context, _ time.Time) (snapshot.PartialSnapshot, error) {
		close(started)
		<-ctx.Done()
		return snapshot.PartialSnapshot{}, ctx.Err()
	})
	runner, _ := NewJobs([]Job{{Collector: collector, Interval: time.Hour, StartupRefresh: true}}, newFakeStore(), newFakeClock(), nil, Config{MaxConcurrency: 1, Backoff: BackoffConfig{Initial: time.Minute, Max: time.Minute, Multiplier: 2}, Observer: observer})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { runner.Run(ctx); close(done) }()
	receive(t, started)
	cancel()
	if event := receive(t, observer.refresh); event.status != "canceled" {
		t.Fatalf("refresh=%#v", event)
	}
	receive(t, done)
	runner.runningMu.Lock()
	running := runner.running[collector.ID()]
	runner.runningMu.Unlock()
	if running {
		t.Fatal("running state leaked")
	}
}

func TestNewJobsRejectsInvalidAndDuplicateIDs(t *testing.T) {
	store, clock := newFakeStore(), newFakeClock()
	collector := newCollector("a", "total", func(context.Context, time.Time) (snapshot.PartialSnapshot, error) {
		return snapshot.PartialSnapshot{}, nil
	})
	if runner, err := NewJobs(nil, store, clock, nil, Config{}); runner != nil || err == nil {
		t.Fatal("accepted empty jobs")
	}
	config := validSchedulerConfig()
	if runner, err := NewJobs([]Job{{Collector: collector, Interval: time.Hour}, {Collector: collector, Interval: time.Hour}}, store, clock, nil, config); runner != nil || err == nil {
		t.Fatal("accepted duplicate CollectorID")
	}
}

func TestRunnerTreatsCachePublishErrorAsCollectorFailure(t *testing.T) {
	store := newFakeStore()
	store.publishErr = errors.New("invalid snapshot")
	observer := newFakeObserver()
	collector := newCollector("a", "total", func(context.Context, time.Time) (snapshot.PartialSnapshot, error) {
		return snapshot.PartialSnapshot{}, nil
	})
	runner, _ := NewJobs([]Job{{Collector: collector, Interval: time.Hour, StartupRefresh: true}}, store, newFakeClock(), nil, Config{
		MaxConcurrency: 1, Backoff: BackoffConfig{Initial: time.Minute, Max: time.Minute, Multiplier: 2}, Observer: observer,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { runner.Run(ctx); close(done) }()
	if event := receive(t, observer.refresh); event.status != "error" {
		t.Fatalf("refresh=%#v", event)
	}
	if receive(t, store.failed) != collector.ID() {
		t.Fatal("publish failure did not record collector failure")
	}
	if receive(t, observer.cacheErrors) != collector.ID() {
		t.Fatal("publish error was not observed")
	}
	cancel()
	receive(t, done)
}

type fakeCollector struct {
	idValue identity.CollectorID
	collect func(context.Context, time.Time) (snapshot.PartialSnapshot, error)
}

func newCollector(target, name string, collect func(context.Context, time.Time) (snapshot.PartialSnapshot, error)) *fakeCollector {
	return &fakeCollector{idValue: id(target, name), collect: collect}
}
func (value *fakeCollector) ID() identity.CollectorID { return value.idValue }
func (value *fakeCollector) Collect(ctx context.Context, now time.Time) (snapshot.PartialSnapshot, error) {
	return value.collect(ctx, now)
}
func id(target, name string) identity.CollectorID {
	return identity.CollectorID{Target: identity.TargetID(target), Name: name}
}

type fakeStore struct {
	published, failed chan identity.CollectorID
	publishErr        error
}

func newFakeStore() *fakeStore {
	return &fakeStore{published: make(chan identity.CollectorID, 8), failed: make(chan identity.CollectorID, 8)}
}
func (value *fakeStore) Publish(id identity.CollectorID, _ snapshot.PartialSnapshot) error {
	if value.publishErr != nil {
		return value.publishErr
	}
	value.published <- id
	return nil
}
func (value *fakeStore) RecordFailure(id identity.CollectorID) error { value.failed <- id; return nil }

type fakeTimer struct {
	delay time.Duration
	fire  chan time.Time
}
type fakeClock struct {
	now    time.Time
	timers chan fakeTimer
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC), timers: make(chan fakeTimer, 16)}
}
func (value *fakeClock) Now() time.Time { return value.now }
func (value *fakeClock) NewTimer(delay time.Duration) Timer {
	fire := make(chan time.Time, 1)
	value.timers <- fakeTimer{delay: delay, fire: fire}
	return &fakeSchedTimer{fire: fire}
}

type fakeSchedTimer struct{ fire chan time.Time }

func (value *fakeSchedTimer) Chan() <-chan time.Time { return value.fire }
func (value *fakeSchedTimer) Stop() bool {
	select {
	case <-value.fire:
	default:
	}
	return true
}
func (value *fakeSchedTimer) Reset(time.Duration) bool { return false }

type refreshEvent struct {
	id     identity.CollectorID
	status string
}
type skipEvent struct {
	id     identity.CollectorID
	reason string
}
type fakeObserver struct {
	refresh     chan refreshEvent
	skipped     chan skipEvent
	cacheErrors chan identity.CollectorID
}

func newFakeObserver() *fakeObserver {
	return &fakeObserver{refresh: make(chan refreshEvent, 8), skipped: make(chan skipEvent, 8), cacheErrors: make(chan identity.CollectorID, 8)}
}
func (value *fakeObserver) ObserveRefresh(id identity.CollectorID, status string, _ time.Duration) {
	value.refresh <- refreshEvent{id, status}
}
func (value *fakeObserver) ObserveSkipped(id identity.CollectorID, reason string) {
	value.skipped <- skipEvent{id, reason}
}
func (value *fakeObserver) ObserveCachePublishError(id identity.CollectorID, _ string) {
	value.cacheErrors <- id
}

func validSchedulerConfig() Config {
	return Config{JitterRatio: .1, MaxConcurrency: 2, Backoff: BackoffConfig{Initial: time.Minute, Max: time.Minute, Multiplier: 2}}
}
func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scheduler event")
		var zero T
		return zero
	}
}
