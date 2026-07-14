package collector_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

var errFactory = errors.New("factory failed")

// stubCollector provides the minimum behavior required by registry tests.
type stubCollector struct {
	name string
}

// Name returns the test collector identifier.
func (stub stubCollector) Name() string {
	return stub.name
}

// Collect returns an empty snapshot for registry contract tests.
func (stub stubCollector) Collect(context.Context, time.Time) (cost.PartialSnapshot, error) {
	return cost.NewSnapshot(nil, nil), nil
}

// TestRegistryRejectsInvalidRegistrations verifies nil and duplicate factories
// cannot silently replace collector implementations.
func TestRegistryRejectsInvalidRegistrations(t *testing.T) {
	t.Parallel()

	registry := collector.NewRegistry()
	if err := registry.Register("", func() (collector.Collector, error) {
		return stubCollector{name: "total"}, nil
	}); !errors.Is(err, collector.ErrInvalidRegistration) {
		t.Fatalf("Register(empty name) error = %v, want ErrInvalidRegistration", err)
	}
	if err := registry.Register("total", nil); !errors.Is(err, collector.ErrInvalidRegistration) {
		t.Fatalf("Register(nil factory) error = %v, want ErrInvalidRegistration", err)
	}

	factory := func() (collector.Collector, error) {
		return stubCollector{name: "total"}, nil
	}
	if err := registry.Register("total", factory); err != nil {
		t.Fatalf("Register() returned an unexpected error: %v", err)
	}
	if err := registry.Register("total", factory); !errors.Is(err, collector.ErrDuplicateRegistration) {
		t.Fatalf("Register(duplicate) error = %v, want ErrDuplicateRegistration", err)
	}
}

// TestRegistryBuildUsesExplicitOrder verifies configuration order remains
// stable regardless of registration order.
func TestRegistryBuildUsesExplicitOrder(t *testing.T) {
	t.Parallel()

	registry := collector.NewRegistry()
	for _, name := range []string{"first", "second"} {
		name := name
		if err := registry.Register(name, func() (collector.Collector, error) {
			return stubCollector{name: name}, nil
		}); err != nil {
			t.Fatalf("Register(%q): %v", name, err)
		}
	}

	built, err := registry.Build([]string{"second", "first"})
	if err != nil {
		t.Fatalf("Build() returned an unexpected error: %v", err)
	}
	if len(built) != 2 || built[0].Name() != "second" || built[1].Name() != "first" {
		t.Fatalf("Build() order = %v, want [second first]", collectorNames(built))
	}
}

// TestRegistryBuildReportsUnknownAndFactoryErrors verifies startup failures
// identify the affected collector without being swallowed.
func TestRegistryBuildReportsUnknownAndFactoryErrors(t *testing.T) {
	t.Parallel()

	registry := collector.NewRegistry()
	if _, err := registry.Build([]string{"missing"}); !errors.Is(err, collector.ErrUnknownCollector) {
		t.Fatalf("Build(unknown) error = %v, want ErrUnknownCollector", err)
	}
	if err := registry.Register("broken", func() (collector.Collector, error) {
		return nil, errFactory
	}); err != nil {
		t.Fatalf("Register() returned an unexpected error: %v", err)
	}
	if _, err := registry.Build([]string{"broken"}); !errors.Is(err, errFactory) {
		t.Fatalf("Build(factory error) error = %v, want wrapped factory error", err)
	}
}

// collectorNames extracts names for readable test failures.
func collectorNames(values []collector.Collector) []string {
	names := make([]string, 0, len(values))
	for _, value := range values {
		names = append(names, value.Name())
	}

	return names
}
