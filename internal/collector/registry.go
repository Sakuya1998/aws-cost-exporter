package collector

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrInvalidRegistration indicates an empty name, nil factory, or invalid product.
	ErrInvalidRegistration = errors.New("invalid collector registration")
	// ErrDuplicateRegistration indicates a collector name is already registered.
	ErrDuplicateRegistration = errors.New("duplicate collector registration")
	// ErrUnknownCollector indicates an enabled collector has no factory.
	ErrUnknownCollector = errors.New("unknown collector")
)

// Registry stores collector factories during application startup.
type Registry struct {
	factories map[string]Factory
}

// NewRegistry constructs an empty collector registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

// Register associates one stable collector name with a factory.
func (registry *Registry) Register(name string, factory Factory) error {
	name = strings.TrimSpace(name)
	if name == "" || factory == nil {
		return ErrInvalidRegistration
	}
	if _, exists := registry.factories[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateRegistration, name)
	}

	registry.factories[name] = factory

	return nil
}

// Build constructs enabled collectors in the exact requested order.
func (registry *Registry) Build(names []string) ([]Collector, error) {
	built := make([]Collector, 0, len(names))
	for _, name := range names {
		factory, exists := registry.factories[name]
		if !exists {
			return nil, fmt.Errorf("%w: %s", ErrUnknownCollector, name)
		}

		instance, err := factory()
		if err != nil {
			return nil, fmt.Errorf("construct collector %s: %w", name, err)
		}
		if instance == nil || instance.Name() != name {
			return nil, fmt.Errorf("%w: factory %s returned mismatched collector", ErrInvalidRegistration, name)
		}
		built = append(built, instance)
	}

	return built, nil
}
