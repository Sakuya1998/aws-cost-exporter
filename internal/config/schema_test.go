package config_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
)

// TestDefaultReturnsExpectedConfig locks every documented default to the
// strongly typed configuration contract.
func TestDefaultReturnsExpectedConfig(t *testing.T) {
	t.Parallel()

	want := config.Config{
		Server: config.ServerConfig{
			ListenAddress: ":8080", MetricsPath: "/metrics",
			ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
			WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
			ShutdownTimeout: 15 * time.Second, MaxInFlight: 20,
			Debug: config.DebugConfig{Enabled: false},
		},
		Log: config.LogConfig{Level: "info", Format: "json", AddSource: false},
		AWS: config.AWSConfig{
			Region: "us-east-1", Profile: "", EndpointURL: "",
			RequestTimeout: 30 * time.Second,
			Retry: config.RetryConfig{
				MaxAttempts: 5, BaseDelay: time.Second, MaxBackoff: 30 * time.Second,
			},
			RateLimit: config.RateLimitConfig{RequestsPerSecond: 0.5, Burst: 1},
		},
		CostExplorer: config.CostExplorerConfig{
			Enabled: true, CostMetric: "UnblendedCost", MaxPages: 50,
			RefreshInterval: 6 * time.Hour, StartupRefresh: true,
			JitterRatio: 0.10,
			Forecast:    config.ForecastConfig{Enabled: true, PredictionInterval: 80},
			Collectors:  config.CollectorsConfig{Total: true, Service: true, Region: true, Account: true},
			Dimensions:  config.DimensionsConfig{SeriesLimit: 1000, Overflow: "aggregate", OverflowLabel: "__other__"},
			Filters: config.FiltersConfig{
				LinkedAccountIDs: []string{}, Services: []string{}, Regions: []string{},
			},
		},
		Cache: config.CacheConfig{
			FreshnessTTL: 12 * time.Hour, StaleAfter: 24 * time.Hour,
		},
		Scheduler: config.SchedulerConfig{
			MaxConcurrency: 2,
			FailureBackoff: config.BackoffConfig{
				Initial: time.Minute, Max: 30 * time.Minute, Multiplier: 2,
			},
		},
		Telemetry: config.TelemetryConfig{
			IncludeGoCollector: true, IncludeProcessCollector: true,
		},
	}

	if got := config.Default(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Default() = %#v, want %#v", got, want)
	}
}

// TestDefaultReturnsIndependentFilters verifies callers cannot mutate filter
// defaults returned by a later call.
func TestDefaultReturnsIndependentFilters(t *testing.T) {
	t.Parallel()

	first := config.Default()
	first.CostExplorer.Filters.Services = append(
		first.CostExplorer.Filters.Services,
		"Amazon EC2",
	)

	second := config.Default()
	if len(second.CostExplorer.Filters.Services) != 0 {
		t.Fatalf("Default() reused mutable filter storage: %v", second.CostExplorer.Filters.Services)
	}
}
