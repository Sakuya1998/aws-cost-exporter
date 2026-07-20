// Package scheduler coordinates periodic collector refreshes.
package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"math"
	rand "math/rand/v2"
	"sync"
	"time"

	basecollector "github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
)

type Collector = basecollector.Collector

// Job binds one collector to its independent periodic schedule.
type Job struct {
	Collector      Collector
	Interval       time.Duration
	StartupRefresh bool
}

type Store interface {
	Publish(identity.CollectorID, snapshot.PartialSnapshot) error
	RecordFailure(identity.CollectorID) error
}

type Clock interface {
	Now() time.Time
	NewTimer(time.Duration) Timer
}

type Observer interface {
	ObserveRefresh(identity.CollectorID, string, time.Duration)
	ObserveSkipped(identity.CollectorID, string)
	ObserveCachePublishError(identity.CollectorID, string)
}

type Config struct {
	Interval       time.Duration
	StartupRefresh bool
	JitterRatio    float64
	MaxConcurrency int
	Backoff        BackoffConfig
	Observer       Observer
	Logger         *slog.Logger
}

type BackoffConfig struct {
	MaxAttempts int
	Initial     time.Duration
	Max         time.Duration
	Multiplier  float64
}

var ErrInvalidConfig = errors.New("invalid scheduler configuration")

// Runner schedules target-scoped jobs with bounded concurrency and single-flight.
type Runner struct {
	jobs      []Job
	store     Store
	clock     Clock
	random    func() float64
	config    Config
	semaphore chan struct{}
	targets   map[identity.TargetID]chan struct{}
	randomMu  sync.Mutex
	runningMu sync.Mutex
	running   map[identity.CollectorID]bool
	queues    map[identity.CollectorID]chan time.Time
	workers   sync.WaitGroup
}

// New preserves the simple same-schedule constructor for internal callers.
func New(collectors []Collector, store Store, clock Clock, random func() float64, config Config) (*Runner, error) {
	jobs := make([]Job, 0, len(collectors))
	for _, collector := range collectors {
		jobs = append(jobs, Job{Collector: collector, Interval: config.Interval, StartupRefresh: config.StartupRefresh})
	}
	return NewJobs(jobs, store, clock, random, config)
}

// NewJobs constructs a runner with independent job intervals.
func NewJobs(jobs []Job, store Store, clock Clock, random func() float64, config Config) (*Runner, error) {
	backoff := config.Backoff
	if len(jobs) == 0 || store == nil || clock == nil || config.MaxConcurrency <= 0 ||
		config.JitterRatio < 0 || config.JitterRatio > 0.5 || math.IsNaN(config.JitterRatio) || math.IsInf(config.JitterRatio, 0) ||
		backoff.MaxAttempts <= 0 || backoff.MaxAttempts > 10 || backoff.Initial <= 0 || backoff.Max < backoff.Initial || backoff.Multiplier <= 1 || math.IsNaN(backoff.Multiplier) || math.IsInf(backoff.Multiplier, 0) {
		return nil, ErrInvalidConfig
	}
	known := make(map[identity.CollectorID]struct{}, len(jobs))
	queues := make(map[identity.CollectorID]chan time.Time, len(jobs))
	targets := make(map[identity.TargetID]chan struct{})
	for _, job := range jobs {
		if job.Collector == nil || job.Interval <= 0 || !job.Collector.ID().Valid() {
			return nil, ErrInvalidConfig
		}
		id := job.Collector.ID()
		if _, duplicate := known[id]; duplicate {
			return nil, ErrInvalidConfig
		}
		known[id] = struct{}{}
		queues[id] = make(chan time.Time, 1)
		if targets[id.Target] == nil {
			targets[id.Target] = make(chan struct{}, 1)
		}
	}
	if random == nil {
		random = rand.Float64
	}
	return &Runner{
		jobs: append([]Job(nil), jobs...), store: store, clock: clock, random: random, config: config,
		semaphore: make(chan struct{}, config.MaxConcurrency), targets: targets,
		running: make(map[identity.CollectorID]bool), queues: queues,
	}, nil
}

// Run starts schedule and collection workers and waits for complete cancellation.
func (runner *Runner) Run(ctx context.Context) {
	runner.workers.Add(2 * len(runner.jobs))
	for _, job := range runner.jobs {
		go runner.schedule(ctx, job)
		go runner.worker(ctx, job)
	}
	<-ctx.Done()
	runner.workers.Wait()
}

func (runner *Runner) schedule(ctx context.Context, job Job) {
	defer runner.workers.Done()
	if job.StartupRefresh {
		runner.dispatch(job.Collector.ID(), runner.clock.Now())
	}
	timer := runner.clock.NewTimer(runner.nextDelay(job.Interval))
	defer stopTimer(timer)
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.Chan():
			runner.dispatch(job.Collector.ID(), runner.clock.Now())
			if !timer.Reset(runner.nextDelay(job.Interval)) {
				stopTimer(timer)
				timer = runner.clock.NewTimer(runner.nextDelay(job.Interval))
			}
		}
	}
}

func (runner *Runner) nextDelay(interval time.Duration) time.Duration {
	runner.randomMu.Lock()
	value := min(max(runner.random(), 0), 1)
	runner.randomMu.Unlock()
	return interval + time.Duration(float64(interval)*runner.config.JitterRatio*value)
}

func (runner *Runner) dispatch(id identity.CollectorID, reference time.Time) {
	runner.runningMu.Lock()
	if runner.running[id] {
		runner.runningMu.Unlock()
		if runner.config.Observer != nil {
			runner.config.Observer.ObserveSkipped(id, "single_flight")
		}
		return
	}
	runner.running[id] = true
	runner.runningMu.Unlock()
	runner.queues[id] <- reference
}

func (runner *Runner) worker(ctx context.Context, job Job) {
	id := job.Collector.ID()
	defer runner.workers.Done()
	defer runner.clearRunning(id)
	for {
		select {
		case reference := <-runner.queues[id]:
			runner.collect(ctx, reference, job)
			runner.clearRunning(id)
		case <-ctx.Done():
			return
		}
	}
}

func (runner *Runner) collect(ctx context.Context, reference time.Time, job Job) {
	delay := runner.config.Backoff.Initial
	id := job.Collector.ID()
	for attempt := 1; attempt <= runner.config.Backoff.MaxAttempts; attempt++ {
		target := runner.targets[id.Target]
		select {
		case target <- struct{}{}:
		case <-ctx.Done():
			return
		}
		select {
		case runner.semaphore <- struct{}{}:
		case <-ctx.Done():
			<-target
			return
		}
		started := runner.clock.Now()
		partial, err := job.Collector.Collect(ctx, reference)
		<-runner.semaphore
		<-target
		if err == nil {
			if publishErr := runner.store.Publish(id, partial); publishErr != nil {
				runner.observeCachePublishError(id, "publish", publishErr)
				err = publishErr
			}
		}
		if runner.config.Observer != nil {
			status := "success"
			if err != nil {
				status = "error"
				if ctx.Err() != nil {
					status = "canceled"
				}
			}
			runner.config.Observer.ObserveRefresh(id, status, runner.clock.Now().Sub(started))
		}
		if err == nil {
			return
		}
		if ctx.Err() != nil {
			return
		}
		var retryable interface{ Retryable() bool }
		canRetry := errors.As(err, &retryable) && retryable.Retryable()
		runner.logCollectorFailure(id, err, canRetry)
		if recordErr := runner.store.RecordFailure(id); recordErr != nil {
			runner.observeCachePublishError(id, "record_failure", recordErr)
		}
		if !canRetry || attempt == runner.config.Backoff.MaxAttempts {
			return
		}
		backoff := runner.clock.NewTimer(delay)
		select {
		case <-backoff.Chan():
			stopTimer(backoff)
			reference = runner.clock.Now()
			delay = runner.nextBackoff(delay)
		case <-ctx.Done():
			stopTimer(backoff)
			return
		}
	}
}

func (runner *Runner) logCollectorFailure(id identity.CollectorID, err error, retryable bool) {
	if runner.config.Logger == nil {
		return
	}
	kind := "unknown"
	var classified interface{ SafeKind() string }
	if errors.As(err, &classified) {
		switch classified.SafeKind() {
		case "canceled", "timeout", "throttle", "authorization", "validation", "transient", "unknown":
			kind = classified.SafeKind()
		}
	}
	runner.config.Logger.Warn("collector refresh failed", "target", id.Target, "collector", id.Name, "error_kind", kind, "retryable", retryable)
}

func (runner *Runner) observeCachePublishError(id identity.CollectorID, operation string, err error) {
	if runner.config.Logger != nil {
		runner.config.Logger.Warn("cache publish failed", "target", id.Target, "collector", id.Name, "operation", operation, "err", err)
	}
	if runner.config.Observer != nil {
		runner.config.Observer.ObserveCachePublishError(id, operation)
	}
}

func (runner *Runner) nextBackoff(delay time.Duration) time.Duration {
	next := float64(delay) * runner.config.Backoff.Multiplier
	if next >= float64(runner.config.Backoff.Max) {
		return runner.config.Backoff.Max
	}
	return time.Duration(next)
}

func (runner *Runner) clearRunning(id identity.CollectorID) {
	runner.runningMu.Lock()
	delete(runner.running, id)
	runner.runningMu.Unlock()
}
