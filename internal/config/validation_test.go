package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
)

// TestValidateEnforcesCrossFieldInvariants verifies invalid values identify
// their exact configuration path.
func TestValidateEnforcesCrossFieldInvariants(t *testing.T) {
	t.Parallel()

	if err := config.Validate(config.Default()); err != nil {
		t.Fatalf("Validate(Default()) returned an unexpected error: %v", err)
	}

	tests := []struct {
		name   string
		path   string
		mutate func(*config.Config)
	}{
		{name: "metrics path", path: "server.metrics_path", mutate: func(value *config.Config) {
			value.Server.MetricsPath = "metrics"
		}},
		{name: "AWS region", path: "aws.region", mutate: func(value *config.Config) {
			value.AWS.Region = "us-west-2"
		}},
		{name: "refresh interval", path: "cost_explorer.refresh_interval", mutate: func(value *config.Config) {
			value.CostExplorer.RefreshInterval = value.AWS.RequestTimeout
		}},
		{name: "jitter", path: "cost_explorer.jitter_ratio", mutate: func(value *config.Config) {
			value.CostExplorer.JitterRatio = 0.6
		}},
		{name: "prediction", path: "cost_explorer.forecast.prediction_interval", mutate: func(value *config.Config) {
			value.CostExplorer.Forecast.PredictionInterval = 79
		}},
		{name: "series limit", path: "cost_explorer.dimensions.series_limit", mutate: func(value *config.Config) {
			value.CostExplorer.Dimensions.SeriesLimit = 0
		}},
		{name: "freshness", path: "cache.freshness_ttl", mutate: func(value *config.Config) {
			value.Cache.FreshnessTTL = time.Hour
		}},
		{name: "staleness", path: "cache.stale_after", mutate: func(value *config.Config) {
			value.Cache.StaleAfter = value.Cache.FreshnessTTL - time.Second
		}},
		{name: "collectors", path: "cost_explorer.collectors", mutate: func(value *config.Config) {
			value.CostExplorer.Collectors = config.CollectorsConfig{}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			value := config.Default()
			test.mutate(&value)
			err := config.Validate(value)
			if err == nil || !strings.Contains(err.Error(), test.path) {
				t.Fatalf("Validate() error = %v, want field path %q", err, test.path)
			}
		})
	}
}

// TestLoadRunsSemanticValidation verifies Check and the runtime loader share
// the same validation path.
func TestLoadRunsSemanticValidation(t *testing.T) {
	t.Parallel()

	options := config.Options{Overrides: map[string]any{"aws.region": "us-west-2"}}
	if _, err := config.Load(options); err == nil || !strings.Contains(err.Error(), "aws.region") {
		t.Fatalf("Load() error = %v, want aws.region validation error", err)
	}
	if err := config.Check(options); err == nil || !strings.Contains(err.Error(), "aws.region") {
		t.Fatalf("Check() error = %v, want aws.region validation error", err)
	}
}
