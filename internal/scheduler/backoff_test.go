package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
)

type retryableTestError struct{}

func (retryableTestError) Error() string   { return "retryable private error" }
func (retryableTestError) Retryable() bool { return true }

func TestRunnerRetriesWithBoundedBackoff(t *testing.T) {
	clock := newFakeClock()
	store := newFakeStore()
	calls := make(chan int, 2)
	count := 0
	collector := newCollector("a", "total", func(context.Context, time.Time) (snapshot.PartialSnapshot, error) {
		count++
		calls <- count
		if count == 1 {
			return snapshot.PartialSnapshot{}, retryableTestError{}
		}
		return snapshot.PartialSnapshot{}, nil
	})
	config := validSchedulerConfig()
	config.JitterRatio = 0
	config.Backoff = BackoffConfig{MaxAttempts: 3, Initial: time.Minute, Max: 2 * time.Minute, Multiplier: 2}
	runner, _ := NewJobs([]Job{{Collector: collector, Interval: time.Hour, StartupRefresh: true}}, store, clock, nil, config)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { runner.Run(ctx); close(done) }()
	receive(t, calls)
	var backoff fakeTimer
	for range 2 {
		timer := receive(t, clock.timers)
		if timer.delay == time.Minute {
			backoff = timer
		}
	}
	if backoff.fire == nil {
		t.Fatal("missing refresh backoff timer")
	}
	backoff.fire <- clock.Now()
	if receive(t, calls) != 2 {
		t.Fatal("missing retry")
	}
	receive(t, store.published)
	cancel()
	receive(t, done)
}
