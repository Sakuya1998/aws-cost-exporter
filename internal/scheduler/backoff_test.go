package scheduler

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

// TestRunnerBackoffCapsAndResetsAfterSuccess verifies one retry chain per collector.
func TestRunnerBackoffCapsAndResetsAfterSuccess(t *testing.T) {
	clock, store := newFakeClock(), newFakeStore()
	attempts := make(chan int, 8)
	attempt := 0
	instance := &fakeCollector{name: "total", collect: func(context.Context, time.Time) (cost.PartialSnapshot, error) {
		attempt++
		attempts <- attempt
		if attempt <= 3 || attempt == 5 {
			return cost.PartialSnapshot{}, retryableFailure{}
		}
		return cost.PartialSnapshot{}, nil
	}}
	subject, err := New([]Collector{instance}, store, clock, func() float64 { return 0 }, Config{
		Interval: time.Hour, StartupRefresh: true, MaxConcurrency: 1,
		Backoff: BackoffConfig{Initial: time.Minute, Max: 3 * time.Minute, Multiplier: 2},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { subject.Run(ctx); close(done) }()

	var periodic fakeTimer
	for number, delay := range []time.Duration{time.Minute, 2 * time.Minute, 3 * time.Minute} {
		if got := receive(t, attempts); got != number+1 {
			t.Fatalf("attempt = %d, want %d", got, number+1)
		}
		receive(t, store.failed)
		timer := timerWithDelay(t, clock, delay, &periodic)
		timer.fire <- clock.Now()
	}
	if got := receive(t, attempts); got != 4 {
		t.Fatalf("attempt = %d, want 4", got)
	}
	receive(t, store.published)
	waitIdle(t, subject, "total")
	periodic.fire <- clock.Now()
	if got := receive(t, attempts); got != 5 {
		t.Fatalf("attempt = %d, want 5", got)
	}
	receive(t, store.failed)
	timerWithDelay(t, clock, time.Minute, nil).fire <- clock.Now()
	if got := receive(t, attempts); got != 6 {
		t.Fatalf("attempt = %d, want 6", got)
	}
	receive(t, store.published)
	cancel()
	receive(t, done)
}

// TestRunnerPermanentFailureWaitsForPeriodAndDoesNotBlockPeers verifies isolation.
func TestRunnerPermanentFailureWaitsForPeriodAndDoesNotBlockPeers(t *testing.T) {
	clock, store := newFakeClock(), newFakeStore()
	permanentCalls := make(chan struct{}, 2)
	failing := &fakeCollector{name: "account", collect: func(context.Context, time.Time) (cost.PartialSnapshot, error) {
		permanentCalls <- struct{}{}
		return cost.PartialSnapshot{}, errors.New("authorization")
	}}
	healthy := &fakeCollector{name: "service", collect: func(context.Context, time.Time) (cost.PartialSnapshot, error) {
		return cost.PartialSnapshot{}, nil
	}}
	subject, _ := New([]Collector{failing, healthy}, store, clock, nil, Config{
		Interval: time.Hour, StartupRefresh: true, MaxConcurrency: 2,
		Backoff: BackoffConfig{Initial: time.Minute, Max: time.Minute, Multiplier: 2},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { subject.Run(ctx); close(done) }()
	receive(t, permanentCalls)
	receive(t, store.failed)
	if name := receive(t, store.published); name != "service" {
		t.Fatalf("published collector = %q, want service", name)
	}
	periodic := receive(t, clock.timers)
	if periodic.delay != time.Hour {
		t.Fatalf("unexpected retry timer %v for permanent failure", periodic.delay)
	}
	select {
	case timer := <-clock.timers:
		t.Fatalf("unexpected additional timer with delay %v", timer.delay)
	case <-time.After(20 * time.Millisecond):
	}
	waitIdle(t, subject, "account")
	periodic.fire <- clock.Now()
	receive(t, permanentCalls)
	receive(t, store.failed)
	if name := receive(t, store.published); name != "service" {
		t.Fatalf("published collector = %q, want service", name)
	}
	cancel()
	receive(t, done)
}

type retryableFailure struct{}

func (retryableFailure) Error() string   { return "transient" }
func (retryableFailure) Retryable() bool { return true }

func timerWithDelay(t *testing.T, clock *fakeClock, want time.Duration, saved *fakeTimer) fakeTimer {
	t.Helper()
	for {
		timer := receive(t, clock.timers)
		if timer.delay == want {
			return timer
		}
		if timer.delay == time.Hour {
			if saved != nil {
				*saved = timer
			}
			continue
		}
		t.Fatalf("timer delay = %v, want %v", timer.delay, want)
	}
}

func waitIdle(t *testing.T, runner *Runner, name string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		runner.runningMu.Lock()
		running := runner.running[name]
		runner.runningMu.Unlock()
		if !running {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("collector %q did not become idle", name)
}
