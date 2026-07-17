// Package service collects service-grouped daily and month-to-date costs.
package service

import (
	"context"
	"errors"
	"time"

	basecollector "github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
)

const (
	// Name is the stable registry and telemetry identifier.
	Name = "service"
)

var (
	// ErrNilReader indicates a missing Cost Explorer dependency.
	ErrNilReader = errors.New("service cost reader must not be nil")
	// ErrInvalidSeriesLimit preserves the collector-specific public contract.
	ErrInvalidSeriesLimit = basecollector.ErrInvalidSeriesLimit
	// ErrInvalidOverflowLabel preserves the collector-specific public contract.
	ErrInvalidOverflowLabel = basecollector.ErrInvalidOverflowLabel
	// ErrMixedCurrency preserves the collector-specific public contract.
	ErrMixedCurrency = basecollector.ErrMixedCurrency
)

// Reader is the narrow cost-reading port required by this collector.
type Reader = basecollector.GroupedReader

// Collector retrieves service-grouped costs under a bounded series budget.
type Collector struct {
	id            identity.CollectorID
	reader        Reader
	seriesLimit   int
	overflowLabel string
	observers     []basecollector.OverflowObserver
}

// New validates dependencies and constructs a service collector.
func New(reader Reader, seriesLimit int, overflowLabel string, observers ...basecollector.OverflowObserver) (*Collector, error) {
	return NewForTarget("default", reader, seriesLimit, overflowLabel, observers...)
}

// NewForTarget constructs a target-scoped service collector.
func NewForTarget(target identity.TargetID, reader Reader, seriesLimit int, overflowLabel string, observers ...basecollector.OverflowObserver) (*Collector, error) {
	if reader == nil {
		return nil, ErrNilReader
	}
	if seriesLimit <= 0 {
		return nil, ErrInvalidSeriesLimit
	}
	if err := basecollector.ValidateOverflowLabel(overflowLabel); err != nil {
		return nil, ErrInvalidOverflowLabel
	}
	return &Collector{
		id:     identity.CollectorID{Target: target, Name: Name},
		reader: reader, seriesLimit: seriesLimit, overflowLabel: overflowLabel,
		observers: append([]basecollector.OverflowObserver(nil), observers...),
	}, nil
}

// Name returns the stable collector identifier.
func (collector *Collector) Name() string { return Name }

// ID returns the target-scoped collector identity.
func (collector *Collector) ID() identity.CollectorID { return collector.id }

// Collect retrieves and cardinality-limits daily and month-to-date costs.
func (collector *Collector) Collect(
	ctx context.Context,
	reference time.Time,
) (snapshot.PartialSnapshot, error) {
	return basecollector.CollectGrouped(
		ctx, reference, collector.id.Target, cost.DimensionService, collector.seriesLimit, collector.overflowLabel,
		collector.reader, nil, nil, collector.observers...,
	)
}
