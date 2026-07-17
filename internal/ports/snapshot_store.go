package ports

import (
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
)

// Freshness is the bounded age category of a collector's last success.
type Freshness string

const (
	FreshnessMissing Freshness = "missing"
	FreshnessFresh   Freshness = "fresh"
	FreshnessAging   Freshness = "aging"
	FreshnessStale   Freshness = "stale"
)

// CollectorStatus describes attempt, success, and age state.
type CollectorStatus struct {
	LastAttempt time.Time
	LastSuccess time.Time
	Up          bool
	Freshness   Freshness
	Series      int
}

// SnapshotView is one isolated read of cache data and collector statuses.
type SnapshotView struct {
	Snapshot   snapshot.Snapshot
	Collectors map[identity.CollectorID]CollectorStatus
}

// SnapshotStore publishes collector results without exposing storage details.
type SnapshotStore interface {
	Publish(identity.CollectorID, snapshot.PartialSnapshot) error
	RecordFailure(identity.CollectorID) error
	Snapshot() snapshot.Snapshot
	Load() SnapshotView
}
