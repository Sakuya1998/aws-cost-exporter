// Package memory provides a lock-free-read, copy-on-write snapshot cache.
package memory

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

var (
	// ErrInvalidConfig indicates invalid cache durations or dependencies.
	ErrInvalidConfig = errors.New("invalid memory cache configuration")
	// ErrInvalidCollector indicates an empty collector identity.
	ErrInvalidCollector = errors.New("collector name must not be empty")
)

type state struct {
	snapshot cost.Snapshot
	parts    map[string]cost.PartialSnapshot
	statuses map[string]ports.CollectorStatus
}

// Store atomically publishes immutable cache states.
type Store struct {
	clock        ports.Clock
	freshnessTTL time.Duration
	staleAfter   time.Duration
	writeMu      sync.Mutex
	current      atomic.Pointer[state]
}

// New validates configuration and initializes an empty cache.
func New(clock ports.Clock, freshnessTTL, staleAfter time.Duration) (*Store, error) {
	if clock == nil || freshnessTTL <= 0 || staleAfter < freshnessTTL {
		return nil, ErrInvalidConfig
	}
	store := &Store{clock: clock, freshnessTTL: freshnessTTL, staleAfter: staleAfter}
	store.current.Store(&state{parts: make(map[string]cost.PartialSnapshot), statuses: make(map[string]ports.CollectorStatus)})
	return store, nil
}

// Publish replaces one collector partial and atomically rebuilds the snapshot.
func (store *Store) Publish(name string, snapshot cost.PartialSnapshot) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidCollector
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	current := store.current.Load()
	parts, statuses := cloneMap(current.parts), cloneMap(current.statuses)
	parts[name] = snapshot
	now := store.clock.Now().UTC()
	statuses[name] = ports.CollectorStatus{
		LastAttempt: now, LastSuccess: now, Up: true,
		Series: len(snapshot.Costs()) + 3*len(snapshot.Forecasts()),
	}
	store.current.Store(&state{snapshot: mergeParts(parts), parts: parts, statuses: statuses})
	return nil
}

// RecordFailure records an attempt while retaining every published partial.
func (store *Store) RecordFailure(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidCollector
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	current := store.current.Load()
	statuses := cloneMap(current.statuses)
	status := statuses[name]
	status.LastAttempt, status.Up = store.clock.Now().UTC(), false
	statuses[name] = status
	store.current.Store(&state{snapshot: current.snapshot, parts: current.parts, statuses: statuses})
	return nil
}

// Snapshot returns the latest complete immutable snapshot without locking.
func (store *Store) Snapshot() cost.Snapshot { return store.current.Load().snapshot }

// Load returns a lock-free snapshot read with an isolated computed status map.
func (store *Store) Load() ports.SnapshotView {
	current, now := store.current.Load(), store.clock.Now().UTC()
	statuses := make(map[string]ports.CollectorStatus, len(current.statuses))
	for name, status := range current.statuses {
		status.Freshness = store.freshness(now, status.LastSuccess)
		statuses[name] = status
	}
	return ports.SnapshotView{Snapshot: current.snapshot, Collectors: statuses}
}

func (store *Store) freshness(now, success time.Time) ports.Freshness {
	if success.IsZero() {
		return ports.FreshnessMissing
	}
	age := now.Sub(success)
	if age <= store.freshnessTTL {
		return ports.FreshnessFresh
	}
	if age <= store.staleAfter {
		return ports.FreshnessAging
	}
	return ports.FreshnessStale
}

func cloneMap[Value any](source map[string]Value) map[string]Value {
	result := make(map[string]Value, len(source)+1)
	for name, value := range source {
		result[name] = value
	}
	return result
}

func mergeParts(parts map[string]cost.PartialSnapshot) cost.Snapshot {
	values := make([]cost.PartialSnapshot, 0, len(parts))
	for _, snapshot := range parts {
		values = append(values, snapshot)
	}
	return cost.MergeSnapshots(values...)
}
