// Package scheduler coordinates periodic collector refreshes.
package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"math"
	rand "math/rand/v2"
	"strings"
	"sync"
	"time"

	basecollector "github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

// Collector is the scheduler's collection dependency.
type Collector = basecollector.Collector

// Store is the narrow publication port required by the runner.
type Store interface {
	Publish(string, cost.PartialSnapshot) error
	RecordFailure(string) error
}

// Clock supplies current time and resettable scheduling timers.
type Clock interface {
	Now() time.Time
	NewTimer(time.Duration) Timer
}

// Observer must be concurrency-safe and return quickly from bounded event calls.
type Observer interface {
	ObserveRefresh(collector, status string, duration time.Duration)
	ObserveSkipped(collector, reason string)
	ObserveCachePublishError(collector, operation string)
}

// Config controls basic periodic scheduling.
type Config struct {
	Interval       time.Duration
	StartupRefresh bool
	JitterRatio    float64
	MaxConcurrency int
	Backoff        BackoffConfig
	Observer       Observer
	Logger         *slog.Logger
}

// BackoffConfig controls collector-level retry delays.
type BackoffConfig struct {
	Initial    time.Duration
	Max        time.Duration
	Multiplier float64
}

// ErrInvalidConfig indicates unsafe scheduler wiring or values.
var ErrInvalidConfig = errors.New("invalid scheduler configuration")

// Runner schedules collectors with bounded concurrency and single-flight.
type Runner struct {
	collectors []Collector
	store      Store
	clock      Clock
	random     func() float64
	config     Config
	semaphore  chan struct{}
	runningMu  sync.Mutex
	running    map[string]bool
	jobs       map[string]chan time.Time
	workers    sync.WaitGroup
}

// New validates dependencies and constructs a runner.
func New(collectors []Collector, store Store, clock Clock, random func() float64, config Config) (*Runner, error) {
	backoff := config.Backoff
	if len(collectors) == 0 || store == nil || clock == nil || config.Interval <= 0 || config.MaxConcurrency <= 0 || config.JitterRatio < 0 || config.JitterRatio > 0.5 || backoff.Initial <= 0 || backoff.Max < backoff.Initial || backoff.Multiplier <= 1 || math.IsNaN(backoff.Multiplier) || math.IsInf(backoff.Multiplier, 0) {
		return nil, ErrInvalidConfig
	}
	names := make(map[string]struct{}, len(collectors))
	jobs := make(map[string]chan time.Time, len(collectors))
	for _, instance := range collectors {
		if instance == nil {
			return nil, ErrInvalidConfig
		}
		name := strings.TrimSpace(instance.Name())
		if _, duplicate := names[name]; name == "" || duplicate {
			return nil, ErrInvalidConfig
		}
		names[name] = struct{}{}
		jobs[name] = make(chan time.Time, 1)
	}
	if random == nil {
		random = rand.Float64
	}
	return &Runner{collectors: append([]Collector(nil), collectors...), store: store, clock: clock, random: random, config: config, semaphore: make(chan struct{}, config.MaxConcurrency), running: make(map[string]bool), jobs: jobs}, nil
}

// Run starts workers once, blocks until cancellation, then waits for them.
func (runner *Runner) Run(ctx context.Context) {
	runner.workers.Add(len(runner.collectors))
	for _, instance := range runner.collectors {
		go runner.worker(ctx, instance)
	}
	if runner.config.StartupRefresh {
		runner.dispatch(runner.clock.Now())
	}
	timer := runner.clock.NewTimer(runner.nextDelay())
	defer stopTimer(timer)
	for {
		select {
		case <-ctx.Done():
			stopTimer(timer)
			runner.workers.Wait()
			return
		case <-timer.Chan():
			runner.dispatch(runner.clock.Now())
			if !timer.Reset(runner.nextDelay()) {
				stopTimer(timer)
				timer = runner.clock.NewTimer(runner.nextDelay())
			}
		}
	}
}

func (runner *Runner) nextDelay() time.Duration {
	value := min(max(runner.random(), 0), 1)
	return runner.config.Interval + time.Duration(float64(runner.config.Interval)*runner.config.JitterRatio*value)
}

func (runner *Runner) dispatch(reference time.Time) {
	for _, instance := range runner.collectors {
		name := instance.Name()
		runner.runningMu.Lock()
		if runner.running[name] {
			runner.runningMu.Unlock()
			if runner.config.Observer != nil {
				runner.config.Observer.ObserveSkipped(name, "single_flight")
			}
			continue
		}
		runner.running[name] = true
		runner.runningMu.Unlock()
		runner.jobs[name] <- reference
	}
}

func (runner *Runner) worker(ctx context.Context, instance Collector) {
	name := instance.Name()
	defer runner.workers.Done()
	for {
		select {
		case reference := <-runner.jobs[name]:
			runner.collect(ctx, reference, instance)
			runner.runningMu.Lock()
			delete(runner.running, name)
			runner.runningMu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

func (runner *Runner) collect(ctx context.Context, reference time.Time, instance Collector) {
	delay := runner.config.Backoff.Initial
	for {
		select {
		case runner.semaphore <- struct{}{}:
		case <-ctx.Done():
			return
		}
		started := runner.clock.Now()
		snapshot, err := instance.Collect(ctx, reference)
		<-runner.semaphore
		if runner.config.Observer != nil {
			status := "success"
			if err != nil {
				status = "error"
				if ctx.Err() != nil {
					status = "canceled"
				}
			}
			runner.config.Observer.ObserveRefresh(instance.Name(), status, runner.clock.Now().Sub(started))
		}
		if err == nil {
			if publishErr := runner.store.Publish(instance.Name(), snapshot); publishErr != nil {
				runner.observeCachePublishError(instance.Name(), "publish", publishErr)
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		name := instance.Name()
		if recordErr := runner.store.RecordFailure(name); recordErr != nil {
			runner.observeCachePublishError(name, "record_failure", recordErr)
		}
		var retryable interface{ Retryable() bool }
		if !errors.As(err, &retryable) || !retryable.Retryable() {
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

func (runner *Runner) observeCachePublishError(collector, operation string, err error) {
	if runner.config.Logger != nil {
		runner.config.Logger.Warn("cache publish failed", "collector", collector, "operation", operation, "err", err)
	}
	if runner.config.Observer != nil {
		runner.config.Observer.ObserveCachePublishError(collector, operation)
	}
}

func (runner *Runner) nextBackoff(delay time.Duration) time.Duration {
	next := float64(delay) * runner.config.Backoff.Multiplier
	if next >= float64(runner.config.Backoff.Max) {
		return runner.config.Backoff.Max
	}
	return time.Duration(next)
}
