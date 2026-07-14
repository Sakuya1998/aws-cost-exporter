package collector

import (
	"errors"
	"fmt"
	"sort"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

var (
	// ErrInvalidSeriesLimit indicates a non-positive cardinality cap.
	ErrInvalidSeriesLimit = errors.New("dimension series limit must be positive")
	// ErrMixedCurrency prevents overflow aggregation across monetary units.
	ErrMixedCurrency = errors.New("dimension overflow contains mixed currencies")
	// ErrReservedDimension prevents collisions with the overflow label.
	ErrReservedDimension = errors.New("reserved dimension value")
)

// OverflowObserver receives bounded dimension overflow counts.
type OverflowObserver interface {
	ObserveOverflow(dimension string, count int)
}

// LimitDimensions keeps the largest values and aggregates overflow while
// preserving the final per-window series budget.
func LimitDimensions(values []cost.Cost, limit int, other string, observers ...OverflowObserver) ([]cost.Cost, error) {
	if limit <= 0 {
		return nil, ErrInvalidSeriesLimit
	}
	for _, value := range values {
		if value.Dimension.Value() == other {
			return nil, ErrReservedDimension
		}
	}
	if len(values) <= limit {
		return append([]cost.Cost(nil), values...), nil
	}
	ranked := append([]cost.Cost(nil), values...)
	sort.SliceStable(ranked, func(left, right int) bool {
		if ranked[left].Amount.Amount() != ranked[right].Amount.Amount() {
			return ranked[left].Amount.Amount() > ranked[right].Amount.Amount()
		}
		return ranked[left].Dimension.Value() < ranked[right].Dimension.Value()
	})

	keep := limit - 1
	currency := ranked[keep].Amount.Currency()
	total := 0.0
	for _, value := range ranked[keep:] {
		if value.Amount.Currency() != currency {
			return nil, ErrMixedCurrency
		}
		total += value.Amount.Amount()
	}
	amount, err := cost.NewMoney(total, currency)
	if err != nil {
		return nil, fmt.Errorf("aggregate dimension overflow: %w", err)
	}
	dimension, err := cost.NewDimension(ranked[keep].Dimension.Kind(), other)
	if err != nil {
		return nil, fmt.Errorf("construct overflow dimension: %w", err)
	}
	overflow := ranked[keep]
	overflow.Dimension, overflow.Amount = dimension, amount
	for _, observer := range observers {
		if observer != nil {
			observer.ObserveOverflow(string(dimension.Kind()), len(values)-keep)
		}
	}

	return append(ranked[:keep:keep], overflow), nil
}
