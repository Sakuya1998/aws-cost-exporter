package config_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
)

func validConfig() config.Config {
	value := config.Default()
	value.AWS.Credentials.Sources = map[string]config.CredentialSourceConfig{
		"runtime": {Type: config.CredentialSourceDefaultChain},
	}
	value.Targets = []config.TargetConfig{{
		Name: "payer-prod", AccountID: "444455556666", Required: true,
		Credentials:  config.TargetCredentialsConfig{Source: "runtime"},
		CostExplorer: config.TargetCostExplorerConfig{Enabled: true},
	}}
	return value
}

func TestDefaultReturnsExpectedV02Shape(t *testing.T) {
	t.Parallel()
	value := config.Default()
	if len(value.Targets) != 0 {
		t.Fatalf("default targets = %v, want explicit empty list", value.Targets)
	}
	if len(value.AWS.Credentials.Sources) != 0 {
		t.Fatalf("default credential sources = %v, want explicit empty map", value.AWS.Credentials.Sources)
	}
	if value.Collection.RefreshInterval != 6*time.Hour || value.Collection.MaxConcurrency != 4 {
		t.Fatalf("collection defaults = %#v", value.Collection)
	}
	if !reflect.DeepEqual(value.AWS.RateLimit, config.RateLimitConfig{GlobalRequestsPerSecond: 1, GlobalBurst: 2, TargetRequestsPerSecond: .5, TargetBurst: 1}) {
		t.Fatalf("rate limit defaults = %#v", value.AWS.RateLimit)
	}
	if value.Collection.CostExplorer.PredictionInterval != 80 || !value.Collection.CostExplorer.Collectors.Forecast {
		t.Fatalf("Cost Explorer defaults = %#v", value.Collection.CostExplorer)
	}
	if err := config.Validate(value); err == nil {
		t.Fatal("Validate(Default()) succeeded without explicit targets")
	}
	if err := config.Validate(validConfig()); err != nil {
		t.Fatalf("Validate(validConfig()) = %v", err)
	}
}

func TestDefaultReturnsIndependentMutableValues(t *testing.T) {
	t.Parallel()
	first := config.Default()
	first.Targets = append(first.Targets, config.TargetConfig{Name: "changed"})
	second := config.Default()
	if len(second.Targets) != 0 {
		t.Fatalf("Default() reused target storage: %v", second.Targets)
	}
}
