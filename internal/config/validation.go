package config

import (
	"fmt"
	"strings"
)

// Validate checks field and cross-field invariants required for safe operation.
func Validate(value Config) error {
	collectors := value.CostExplorer.Collectors
	anyCollector := collectors.Total || collectors.Service || collectors.Region || collectors.Account
	checks := []struct {
		invalid bool
		path    string
		message string
	}{
		{value.Server.ListenAddress == "", "server.listen_address", "must not be empty"},
		{!strings.HasPrefix(value.Server.MetricsPath, "/"), "server.metrics_path", "must start with /"},
		{value.Server.MaxInFlight <= 0, "server.max_in_flight", "must be positive"},
		{value.AWS.Region != "us-east-1", "aws.region", "Cost Explorer requires us-east-1"},
		{value.AWS.RequestTimeout <= 0, "aws.request_timeout", "must be positive"},
		{value.AWS.Retry.MaxAttempts <= 0, "aws.retry.max_attempts", "must be positive"},
		{value.AWS.Retry.BaseDelay <= 0, "aws.retry.base_delay", "must be positive"},
		{value.AWS.Retry.MaxBackoff < value.AWS.Retry.BaseDelay, "aws.retry.max_backoff", "must not be less than base_delay"},
		{value.AWS.RateLimit.RequestsPerSecond <= 0, "aws.rate_limit.requests_per_second", "must be positive"},
		{value.AWS.RateLimit.RequestsPerSecond > 1, "aws.rate_limit.requests_per_second", "must not exceed 1"},
		{value.AWS.RateLimit.Burst <= 0, "aws.rate_limit.burst", "must be positive"},
		{value.CostExplorer.MaxPages <= 0, "cost_explorer.max_pages", "must be positive"},
		{value.CostExplorer.MaxPages > 200, "cost_explorer.max_pages", "must not exceed 200"},
		{value.CostExplorer.CostMetric != "UnblendedCost", "cost_explorer.cost_metric", "only UnblendedCost is supported"},
		{value.CostExplorer.RefreshInterval <= value.AWS.RequestTimeout, "cost_explorer.refresh_interval", "must exceed aws.request_timeout"},
		{value.CostExplorer.JitterRatio < 0 || value.CostExplorer.JitterRatio > 0.5, "cost_explorer.jitter_ratio", "must be between 0 and 0.5"},
		{value.CostExplorer.Forecast.Enabled && (value.CostExplorer.Forecast.PredictionInterval < 80 || value.CostExplorer.Forecast.PredictionInterval > 99), "cost_explorer.forecast.prediction_interval", "must be between 80 and 99"},
		{!anyCollector, "cost_explorer.collectors", "at least one collector must be enabled"},
		{value.CostExplorer.Dimensions.SeriesLimit <= 0, "cost_explorer.dimensions.series_limit", "must be positive"},
		{value.CostExplorer.Dimensions.SeriesLimit > 2000, "cost_explorer.dimensions.series_limit", "must not exceed 2000"},
		{value.CostExplorer.Dimensions.Overflow != "aggregate", "cost_explorer.dimensions.overflow", "only aggregate is supported"},
		{value.Cache.FreshnessTTL < value.CostExplorer.RefreshInterval, "cache.freshness_ttl", "must not be less than refresh_interval"},
		{value.Cache.StaleAfter < value.Cache.FreshnessTTL, "cache.stale_after", "must not be less than freshness_ttl"},
		{value.Scheduler.MaxConcurrency <= 0, "scheduler.max_concurrency", "must be positive"},
		{value.Scheduler.FailureBackoff.Initial <= 0, "scheduler.failure_backoff.initial", "must be positive"},
		{value.Scheduler.FailureBackoff.Max < value.Scheduler.FailureBackoff.Initial, "scheduler.failure_backoff.max", "must not be less than initial"},
		{value.Scheduler.FailureBackoff.Multiplier <= 1, "scheduler.failure_backoff.multiplier", "must exceed 1"},
	}

	for _, check := range checks {
		if check.invalid {
			return fmt.Errorf("%s: %s", check.path, check.message)
		}
	}

	return nil
}
