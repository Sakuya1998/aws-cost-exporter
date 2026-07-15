// Package account collects linked-account daily and month-to-date costs.
package account

import (
	"context"
	"errors"
	"time"

	basecollector "github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

// Name is the stable registry and telemetry identifier.
const Name = "account"

var (
	// ErrNilReader indicates a missing Cost Explorer dependency.
	ErrNilReader = errors.New("account cost reader must not be nil")
	// ErrInvalidAccountID indicates a value is not a 12-digit AWS account ID.
	ErrInvalidAccountID = errors.New("invalid linked account ID")
	// ErrInvalidSeriesLimit preserves the collector-specific public contract.
	ErrInvalidSeriesLimit = basecollector.ErrInvalidSeriesLimit
	// ErrInvalidOverflowLabel preserves the collector-specific public contract.
	ErrInvalidOverflowLabel = basecollector.ErrInvalidOverflowLabel
)

// Reader is the narrow cost-reading port required by this collector.
type Reader = basecollector.GroupedReader

// Collector retrieves validated linked-account costs.
type Collector struct {
	reader           Reader
	linkedAccountIDs []string
	seriesLimit      int
	overflowLabel    string
	observers        []basecollector.OverflowObserver
}

// New validates and copies dependencies and the optional account allowlist.
func New(reader Reader, linkedAccountIDs []string, seriesLimit int, overflowLabel string, observers ...basecollector.OverflowObserver) (*Collector, error) {
	if reader == nil {
		return nil, ErrNilReader
	}
	if seriesLimit <= 0 {
		return nil, ErrInvalidSeriesLimit
	}
	if err := basecollector.ValidateOverflowLabel(overflowLabel); err != nil {
		return nil, ErrInvalidOverflowLabel
	}
	for _, accountID := range linkedAccountIDs {
		if !validAccountID(accountID) {
			return nil, ErrInvalidAccountID
		}
	}
	return &Collector{
		reader: reader, linkedAccountIDs: append([]string(nil), linkedAccountIDs...),
		seriesLimit: seriesLimit, overflowLabel: overflowLabel,
		observers: append([]basecollector.OverflowObserver(nil), observers...),
	}, nil
}

// Name returns the stable collector identifier.
func (collector *Collector) Name() string { return Name }

// Collect retrieves daily and month-to-date account costs atomically.
func (collector *Collector) Collect(
	ctx context.Context,
	reference time.Time,
) (cost.PartialSnapshot, error) {
	return basecollector.CollectGrouped(
		ctx, reference, cost.DimensionAccount, collector.seriesLimit, collector.overflowLabel,
		collector.reader,
		func(query *ports.CostQuery) {
			query.LinkedAccountIDs = append([]string(nil), collector.linkedAccountIDs...)
		},
		validateAccountCosts,
		collector.observers...,
	)
}

// validateAccountCosts rejects malformed provider dimensions without
// including their values in errors.
func validateAccountCosts(values []cost.Cost) error {
	for _, value := range values {
		if value.Dimension.Kind() != cost.DimensionAccount ||
			!validAccountID(value.Dimension.Value()) {
			return ErrInvalidAccountID
		}
	}
	return nil
}

// validAccountID accepts exactly twelve ASCII decimal digits.
func validAccountID(value string) bool {
	if len(value) != 12 {
		return false
	}
	for index := range value {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}
