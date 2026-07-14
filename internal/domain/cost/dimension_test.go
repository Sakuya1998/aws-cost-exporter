package cost_test

import (
	"errors"
	"testing"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

// TestNewDimensionNormalizesValues verifies each supported dimension has a
// stable metric representation.
func TestNewDimensionNormalizesValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		kind      cost.DimensionKind
		value     string
		wantValue string
	}{
		{name: "total", kind: cost.DimensionTotal, wantValue: ""},
		{name: "service", kind: cost.DimensionService, value: " Amazon EC2 ", wantValue: "Amazon EC2"},
		{name: "global region", kind: cost.DimensionRegion, value: " ", wantValue: "global"},
		{name: "account", kind: cost.DimensionAccount, value: "123456789012", wantValue: "123456789012"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dimension, err := cost.NewDimension(test.kind, test.value)
			if err != nil {
				t.Fatalf("NewDimension() returned an unexpected error: %v", err)
			}
			if dimension.Kind() != test.kind || dimension.Value() != test.wantValue {
				t.Fatalf("NewDimension() = (%q, %q), want (%q, %q)", dimension.Kind(), dimension.Value(), test.kind, test.wantValue)
			}
		})
	}
}

// TestNewDimensionRejectsInvalidValues verifies unsupported kinds, missing
// grouped values, and labeled totals cannot enter snapshots.
func TestNewDimensionRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind  cost.DimensionKind
		value string
	}{
		{kind: "unsupported", value: "value"},
		{kind: cost.DimensionService, value: " "},
		{kind: cost.DimensionAccount, value: ""},
		{kind: cost.DimensionTotal, value: "unexpected"},
	}

	for _, test := range tests {
		if _, err := cost.NewDimension(test.kind, test.value); !errors.Is(err, cost.ErrInvalidDimension) {
			t.Errorf("NewDimension(%q, %q) error = %v, want ErrInvalidDimension", test.kind, test.value, err)
		}
	}
}
