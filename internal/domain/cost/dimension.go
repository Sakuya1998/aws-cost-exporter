package cost

import (
	"errors"
	"fmt"
	"strings"
)

// DimensionKind identifies the bounded grouping applied to a cost.
type DimensionKind string

const (
	// DimensionTotal represents an ungrouped account total.
	DimensionTotal DimensionKind = "total"
	// DimensionService groups cost by AWS service.
	DimensionService DimensionKind = "service"
	// DimensionRegion groups cost by AWS billing region.
	DimensionRegion DimensionKind = "region"
	// DimensionAccount groups cost by linked AWS account.
	DimensionAccount DimensionKind = "account"
)

// ErrInvalidDimension indicates an unsupported kind or invalid value.
var ErrInvalidDimension = errors.New("invalid cost dimension")

// Dimension is a normalized cost grouping key.
type Dimension struct {
	kind  DimensionKind
	value string
}

// NewDimension validates and normalizes a cost grouping key.
func NewDimension(kind DimensionKind, value string) (Dimension, error) {
	value = strings.TrimSpace(value)

	switch kind {
	case DimensionTotal:
		if value != "" {
			return Dimension{}, fmt.Errorf("%w: total must not have a value", ErrInvalidDimension)
		}
	case DimensionRegion:
		if value == "" {
			value = "global"
		}
	case DimensionService, DimensionAccount:
		if value == "" {
			return Dimension{}, fmt.Errorf("%w: %s value must not be empty", ErrInvalidDimension, kind)
		}
	default:
		return Dimension{}, fmt.Errorf("%w: unsupported kind %q", ErrInvalidDimension, kind)
	}

	return Dimension{kind: kind, value: value}, nil
}

// Kind returns the dimension category.
func (dimension Dimension) Kind() DimensionKind {
	return dimension.kind
}

// Value returns the normalized dimension value.
func (dimension Dimension) Value() string {
	return dimension.value
}
