package collector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

// DefaultOverflowLabel is the reserved aggregate label for bounded dimensions.
const DefaultOverflowLabel = "__other__"

var (
	// ErrInvalidOverflowLabel indicates an empty or unsafe overflow label.
	ErrInvalidOverflowLabel = errors.New("overflow label must not be empty")
)

// GroupedReader loads grouped Cost Explorer costs for one query.
type GroupedReader interface {
	ReadCosts(context.Context, ports.CostQuery) ([]cost.Cost, error)
}

// ValidateOverflowLabel rejects empty overflow labels.
func ValidateOverflowLabel(label string) error {
	if strings.TrimSpace(label) == "" {
		return ErrInvalidOverflowLabel
	}
	return nil
}

// CollectGrouped retrieves daily and month-to-date costs for one dimension family.
func CollectGrouped(
	ctx context.Context,
	reference time.Time,
	groupBy cost.DimensionKind,
	seriesLimit int,
	overflowLabel string,
	reader GroupedReader,
	mutate func(*ports.CostQuery),
	validate func([]cost.Cost) error,
	observers ...OverflowObserver,
) (cost.PartialSnapshot, error) {
	if err := ValidateOverflowLabel(overflowLabel); err != nil {
		return cost.PartialSnapshot{}, err
	}
	queries, err := BuildDailyAndMTDQueries(reference, groupBy)
	if err != nil {
		return cost.PartialSnapshot{}, err
	}
	var collected []cost.Cost
	for index := range queries {
		if mutate != nil {
			mutate(&queries[index])
		}
		query := queries[index]
		if err := ctx.Err(); err != nil {
			return cost.PartialSnapshot{}, err
		}
		values, err := reader.ReadCosts(ctx, query)
		if err != nil {
			return cost.PartialSnapshot{}, fmt.Errorf("collect %s %s cost: %w", query.Window, groupBy, err)
		}
		if validate != nil {
			if err := validate(values); err != nil {
				return cost.PartialSnapshot{}, fmt.Errorf("validate %s %s cost: %w", query.Window, groupBy, err)
			}
		}
		values, err = LimitDimensions(values, seriesLimit, overflowLabel, observers...)
		if err != nil {
			return cost.PartialSnapshot{}, fmt.Errorf("limit %s %s cost: %w", query.Window, groupBy, err)
		}
		collected = append(collected, values...)
	}
	return cost.NewSnapshot(collected, nil), nil
}
