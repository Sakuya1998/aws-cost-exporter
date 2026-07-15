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

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

// TestRunnerRefreshesAtStartupAndJitteredIntervals verifies periodic publication.
func TestRunnerRefreshesAtStartupAndJitteredIntervals(t *testing.T) {
	clock := newFakeClock()
	calls := make(chan struct{}, 2)
	store := newFakeStore()
	subject, _ := New([]Collector{&fakeCollector{name: "total", collect: func(context.Context, time.Time) (cost.PartialSnapshot, error) {
		calls <- struct{}{}
		return cost.PartialSnapshot{}, nil
	}}}, store, clock, func() float64 { return 0.5 }, Config{
		Interval: 10 * time.Minute, StartupRefresh: true, JitterRatio: 0.1, MaxConcurrency: 1,
		Backoff: BackoffConfig{Initial: time.Minute, Max: time.Minute, Multiplier: 2},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { subject.Run(ctx); close(done) }()
	receive(t, calls)
	receive(t, store.published)
	timer := receive(t, clock.timers)
	if timer.delay != 10*time.Minute+30*time.Second {
		t.Fatalf("timer delay = %v, want 10m30s", timer.delay)
	}
	timer.fire <- clock.Now()
	receive(t, calls)
	receive(t, store.published)
	cancel()
	receive(t, done)
}

// TestRunnerEnforcesConcurrencyAndSingleFlight verifies bounded overlapping runs.
func TestRunnerEnforcesConcurrencyAndSingleFlight(t *testing.T) {
	clock := newFakeClock()
	started, release := make(chan string, 2), make(chan struct{}, 2)
	var mu sync.Mutex
	calls, active, maximum := map[string]int{}, 0, 0
	makeCollector := func(name string) Collector {
		return &fakeCollector{name: name, collect: func(context.Context, time.Time) (cost.PartialSnapshot, error) {
			mu.Lock()
			calls[name]++
			active++
			if active > maximum {
				maximum = active
			}
			mu.Unlock()
			started <- name
			<-release
			mu.Lock()
			active--
			mu.Unlock()
			return cost.PartialSnapshot{}, nil
		}}
	}
	store := newFakeStore()
	observer := newFakeObserver()
	subject, _ := New(
		[]Collector{makeCollector("one"), makeCollector("two")},
		store, clock, func() float64 { return 0 }, Config{
			Interval: time.Hour, StartupRefresh: true, MaxConcurrency: 1,
			Backoff:  BackoffConfig{Initial: time.Minute, Max: time.Minute, Multiplier: 2},
			Observer: observer,
		},
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { subject.Run(ctx); close(done) }()
	receive(t, started)
	receive(t, clock.timers).fire <- clock.Now()
	receive(t, clock.timers)
	for range 2 {
		if event := receive(t, observer.skipped); event.reason != "single_flight" {
			t.Fatalf("skip event = %#v", event)
		}
	}
	release <- struct{}{}
	receive(t, started)
	release <- struct{}{}
	receive(t, store.published)
	receive(t, store.published)
	for range 2 {
		if event := receive(t, observer.refresh); event.status != "success" {
			t.Fatalf("refresh event = %#v", event)
		}
	}
	cancel()
	receive(t, done)
	mu.Lock()
	defer mu.Unlock()
	if calls["one"] != 1 || calls["two"] != 1 || maximum != 1 {
		t.Fatalf("calls=%v maximum=%d, want one each and maximum 1", calls, maximum)
	}
}

// TestRunnerRecordsFailuresAndRejectsInvalidConfig verifies safe failures.
func TestRunnerRecordsFailuresAndRejectsInvalidConfig(t *testing.T) {
	store := newFakeStore()
	observer := newFakeObserver()
	var logs bytes.Buffer
	subject, _ := New([]Collector{&fakeCollector{
		name: "forecast", collect: func(context.Context, time.Time) (cost.PartialSnapshot, error) {
			return cost.PartialSnapshot{}, errors.New("unavailable")
		},
	}}, store, newFakeClock(), nil, Config{
		Interval: time.Hour, StartupRefresh: true, MaxConcurrency: 1,
		Backoff:  BackoffConfig{Initial: time.Minute, Max: time.Minute, Multiplier: 2},
		Observer: observer,
		Logger:   slog.New(slog.NewJSONHandler(&logs, nil)),
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { subject.Run(ctx); close(done) }()
	if name := receive(t, store.failed); name != "forecast" {
		t.Fatalf("failed collector = %q, want forecast", name)
	}
	if event := receive(t, observer.refresh); event.status != "error" {
		t.Fatalf("refresh event = %#v, want error", event)
	}
	if text := logs.String(); !strings.Contains(text, `"msg":"collector refresh failed"`) ||
		!strings.Contains(text, `"collector":"forecast"`) ||
		!strings.Contains(text, `"error_kind":"unknown"`) ||
		strings.Contains(text, "unavailable") {
		t.Fatalf("collector failure log is missing bounded fields or leaked error text: %s", text)
	}
	cancel()
	receive(t, done)
	if runner, err := New(nil, store, nil, nil, Config{}); runner != nil || err == nil {
		t.Fatalf("New(invalid) = %#v, %v; want error", runner, err)
	}
}

// TestRunnerObservesCanceledAttempts verifies shutdown status classification.
func TestRunnerObservesCanceledAttempts(t *testing.T) {
	started := make(chan struct{})
	observer := newFakeObserver()
	instance := &fakeCollector{name: "total", collect: func(ctx context.Context, _ time.Time) (cost.PartialSnapshot, error) {
		close(started)
		<-ctx.Done()
		return cost.PartialSnapshot{}, ctx.Err()
	}}
	subject, _ := New([]Collector{instance}, newFakeStore(), newFakeClock(), nil, Config{
		Interval: time.Hour, StartupRefresh: true, MaxConcurrency: 1,
		Backoff:  BackoffConfig{Initial: time.Minute, Max: time.Minute, Multiplier: 2},
		Observer: observer,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { subject.Run(ctx); close(done) }()
	receive(t, started)
	cancel()
	if event := receive(t, observer.refresh); event.status != "canceled" {
		t.Fatalf("refresh event = %#v, want canceled", event)
	}
	receive(t, done)
	subject.runningMu.Lock()
	_, running := subject.running["total"]
	subject.runningMu.Unlock()
	if running {
		t.Fatal("running flag not cleared after worker shutdown")
	}
}

type fakeCollector struct {
	name    string
	collect func(context.Context, time.Time) (cost.PartialSnapshot, error)
}

func (collector *fakeCollector) Name() string { return collector.name }
func (collector *fakeCollector) Collect(ctx context.Context, now time.Time) (cost.PartialSnapshot, error) {
	return collector.collect(ctx, now)
}

type fakeStore struct {
	published chan string
	failed    chan string
}

func newFakeStore() *fakeStore {
	return &fakeStore{published: make(chan string, 8), failed: make(chan string, 8)}
}
func (store *fakeStore) Publish(name string, _ cost.PartialSnapshot) error {
	store.published <- name
	return nil
}
func (store *fakeStore) RecordFailure(name string) error {
	store.failed <- name
	return nil
}

type fakeTimer struct {
	delay time.Duration
	fire  chan time.Time
}

type refreshEvent struct{ name, status string }
type skipEvent struct{ name, reason string }
type fakeObserver struct {
	refresh chan refreshEvent
	skipped chan skipEvent
}

func newFakeObserver() *fakeObserver {
	return &fakeObserver{refresh: make(chan refreshEvent, 4), skipped: make(chan skipEvent, 4)}
}
func (observer *fakeObserver) ObserveRefresh(name, status string, _ time.Duration) {
	observer.refresh <- refreshEvent{name, status}
}
func (observer *fakeObserver) ObserveSkipped(name, reason string) {
	observer.skipped <- skipEvent{name, reason}
}
func (observer *fakeObserver) ObserveCachePublishError(string, string) {}

type fakeClock struct {
	now    time.Time
	timers chan fakeTimer
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC), timers: make(chan fakeTimer, 4)}
}
func (clock *fakeClock) Now() time.Time { return clock.now }
func (clock *fakeClock) NewTimer(delay time.Duration) Timer {
	fire := make(chan time.Time, 1)
	clock.timers <- fakeTimer{delay: delay, fire: fire}
	return &fakeSchedTimer{fire: fire}
}

type fakeSchedTimer struct {
	fire chan time.Time
}

func (timer *fakeSchedTimer) Chan() <-chan time.Time { return timer.fire }
func (timer *fakeSchedTimer) Stop() bool {
	select {
	case <-timer.fire:
	default:
	}
	return true
}
func (timer *fakeSchedTimer) Reset(time.Duration) bool { return false }

func receive[Value any](t *testing.T, channel <-chan Value) Value {
	t.Helper()
	select {
	case value := <-channel:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scheduler event")
		var zero Value
		return zero
	}
}
