// Package memory provides a lock-free-read, copy-on-write snapshot cache.
package memory

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/organization"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

var (
	ErrInvalidConfig    = errors.New("invalid memory cache configuration")
	ErrInvalidCollector = errors.New("collector identity must be valid")
	ErrSeriesLimit      = errors.New("organization metadata series limit exceeded")
)

// OrganizationPolicy bounds metadata exposure for one target. A non-empty
// allowlist wins; otherwise account IDs are derived from account-cost records.
type OrganizationPolicy struct {
	AccountIDs  []string
	SeriesLimit int
}

// Option configures optional aggregate behavior.
type Option func(*Store) error

// WithOrganizationPolicies installs immutable target metadata policies.
func WithOrganizationPolicies(values map[identity.TargetID]OrganizationPolicy) Option {
	return func(store *Store) error {
		store.organizationPolicies = make(map[identity.TargetID]OrganizationPolicy, len(values))
		for target, policy := range values {
			if target == "" || policy.SeriesLimit <= 0 {
				return ErrInvalidConfig
			}
			policy.AccountIDs = append([]string(nil), policy.AccountIDs...)
			store.organizationPolicies[target] = policy
		}
		return nil
	}
}

type state struct {
	snapshot snapshot.Snapshot
	parts    map[identity.CollectorID]snapshot.PartialSnapshot
	statuses map[identity.CollectorID]ports.CollectorStatus
}

// Store atomically publishes immutable cache states.
type Store struct {
	clock                ports.Clock
	freshnessTTL         time.Duration
	staleAfter           time.Duration
	organizationPolicies map[identity.TargetID]OrganizationPolicy
	writeMu              sync.Mutex
	current              atomic.Pointer[state]
}

// New validates configuration and initializes an empty cache.
func New(clock ports.Clock, freshnessTTL, staleAfter time.Duration, options ...Option) (*Store, error) {
	if clock == nil || freshnessTTL <= 0 || staleAfter < freshnessTTL {
		return nil, ErrInvalidConfig
	}
	store := &Store{clock: clock, freshnessTTL: freshnessTTL, staleAfter: staleAfter}
	for _, option := range options {
		if option == nil || option(store) != nil {
			return nil, ErrInvalidConfig
		}
	}
	store.current.Store(&state{
		parts:    make(map[identity.CollectorID]snapshot.PartialSnapshot),
		statuses: make(map[identity.CollectorID]ports.CollectorStatus),
	})
	return store, nil
}

// Publish replaces one collector partial and atomically rebuilds the snapshot.
func (store *Store) Publish(id identity.CollectorID, partial snapshot.PartialSnapshot) error {
	if !id.Valid() {
		return ErrInvalidCollector
	}
	if err := partial.ValidatePartial(id.Target); err != nil {
		return err
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	current := store.current.Load()
	parts, statuses := cloneParts(current.parts), cloneStatuses(current.statuses)
	parts[id] = partial
	merged, err := store.mergeParts(parts)
	if err != nil {
		return err
	}
	now := store.clock.Now().UTC()
	statuses[id] = ports.CollectorStatus{
		LastAttempt: now, LastSuccess: now, Up: true,
	}
	for partID, part := range parts {
		status := statuses[partID]
		status.Series = publishedSeries(partID.Target, part, merged)
		statuses[partID] = status
	}
	store.current.Store(&state{snapshot: merged, parts: parts, statuses: statuses})
	return nil
}

// RecordFailure records an attempt while retaining every published partial.
func (store *Store) RecordFailure(id identity.CollectorID) error {
	if !id.Valid() {
		return ErrInvalidCollector
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	current := store.current.Load()
	statuses := cloneStatuses(current.statuses)
	status := statuses[id]
	status.LastAttempt, status.Up = store.clock.Now().UTC(), false
	statuses[id] = status
	store.current.Store(&state{snapshot: current.snapshot, parts: current.parts, statuses: statuses})
	return nil
}

// Snapshot returns the latest complete immutable snapshot without locking.
func (store *Store) Snapshot() snapshot.Snapshot { return store.current.Load().snapshot }

// Load returns a lock-free snapshot read with an isolated computed status map.
func (store *Store) Load() ports.SnapshotView {
	current, now := store.current.Load(), store.clock.Now().UTC()
	statuses := make(map[identity.CollectorID]ports.CollectorStatus, len(current.statuses))
	for id, status := range current.statuses {
		status.Freshness = store.freshness(now, status.LastSuccess)
		statuses[id] = status
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

func cloneParts(source map[identity.CollectorID]snapshot.PartialSnapshot) map[identity.CollectorID]snapshot.PartialSnapshot {
	result := make(map[identity.CollectorID]snapshot.PartialSnapshot, len(source)+1)
	for id, value := range source {
		result[id] = value
	}
	return result
}

func cloneStatuses(source map[identity.CollectorID]ports.CollectorStatus) map[identity.CollectorID]ports.CollectorStatus {
	result := make(map[identity.CollectorID]ports.CollectorStatus, len(source)+1)
	for id, value := range source {
		result[id] = value
	}
	return result
}

func (store *Store) mergeParts(parts map[identity.CollectorID]snapshot.PartialSnapshot) (snapshot.Snapshot, error) {
	values := make([]snapshot.PartialSnapshot, 0, len(parts))
	for _, part := range parts {
		values = append(values, part)
	}
	merged := snapshot.Merge(values...)
	if err := merged.ValidateUnique(); err != nil {
		return snapshot.Snapshot{}, err
	}
	if len(store.organizationPolicies) == 0 {
		return merged, nil
	}
	observed := make(map[identity.TargetID]map[string]struct{})
	merged.ForEachCost(func(value cost.Cost) {
		if value.Dimension.Kind() != cost.DimensionAccount || !accountID(value.Dimension.Value()) {
			return
		}
		if observed[value.Target] == nil {
			observed[value.Target] = make(map[string]struct{})
		}
		observed[value.Target][value.Dimension.Value()] = struct{}{}
	})
	allowed := make(map[identity.TargetID]map[string]struct{}, len(store.organizationPolicies))
	for target, policy := range store.organizationPolicies {
		if len(policy.AccountIDs) == 0 {
			allowed[target] = observed[target]
			continue
		}
		set := make(map[string]struct{}, len(policy.AccountIDs))
		for _, id := range policy.AccountIDs {
			set[id] = struct{}{}
		}
		allowed[target] = set
	}
	selected := make([]organization.Account, 0)
	counts := make(map[identity.TargetID]int)
	merged.ForEachAccount(func(value organization.Account) {
		set, configured := allowed[value.Target]
		if !configured {
			return
		}
		if _, include := set[value.AccountID]; !include {
			return
		}
		counts[value.Target]++
		selected = append(selected, value)
	})
	for target, count := range counts {
		if count > store.organizationPolicies[target].SeriesLimit {
			return snapshot.Snapshot{}, ErrSeriesLimit
		}
	}
	result := snapshot.NewWithData(merged.Costs(), merged.Forecasts(), merged.Budgets(), selected, merged.Commitments(), merged.Anomalies(), merged.TagCosts())
	if err := result.ValidateUnique(); err != nil {
		return snapshot.Snapshot{}, err
	}
	return result, nil
}

func accountID(value string) bool {
	if len(value) != 12 {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func publishedSeries(target identity.TargetID, partial snapshot.PartialSnapshot, merged snapshot.Snapshot) int {
	series, rawAccounts, selectedAccounts := partial.SeriesCount(), 0, 0
	partial.ForEachAccount(func(organization.Account) { rawAccounts++ })
	if rawAccounts == 0 {
		return series
	}
	merged.ForEachAccount(func(value organization.Account) {
		if value.Target == target {
			selectedAccounts++
		}
	})
	return series - rawAccounts + selectedAccounts
}
