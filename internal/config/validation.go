package config

import (
	"fmt"
	"math"
	"strings"
)

const (
	maxRetryAttempts  = 10
	maxRateLimitBurst = 5
)

type validationCheck struct {
	invalid bool
	path    string
	message string
}

// ValidateServer checks the complete HTTP server configuration contract.
func ValidateServer(value ServerConfig) error {
	checks := []validationCheck{
		{strings.TrimSpace(value.ListenAddress) == "", "server.listen_address", "must not be empty"},
		{!strings.HasPrefix(value.MetricsPath, "/"), "server.metrics_path", "must start with /"},
		{reservedMetricsPath(value), "server.metrics_path", "conflicts with a reserved HTTP route"},
		{value.MaxInFlight <= 0, "server.max_in_flight", "must be positive"},
		{value.ReadHeaderTimeout <= 0, "server.read_header_timeout", "must be positive"},
		{value.ReadTimeout <= 0, "server.read_timeout", "must be positive"},
		{value.WriteTimeout <= 0, "server.write_timeout", "must be positive"},
		{value.IdleTimeout <= 0, "server.idle_timeout", "must be positive"},
		{value.ShutdownTimeout <= 0, "server.shutdown_timeout", "must be positive"},
	}
	return firstValidationError(checks)
}

func reservedMetricsPath(value ServerConfig) bool {
	path := value.MetricsPath
	if path == "/" || path == "/healthz" || path == "/ready" || path == "/version" {
		return true
	}
	return value.Debug.Enabled &&
		(path == "/debug" || path == "/debug/pprof" || strings.HasPrefix(path, "/debug/pprof/"))
}

// Validate checks field and cross-field invariants required for safe operation.
func Validate(value Config) error {
	if err := ValidateServer(value.Server); err != nil {
		return err
	}
	collectors := value.CostExplorer.Collectors
	anyCollector := collectors.Total || collectors.Service || collectors.Region ||
		collectors.Account || value.CostExplorer.Forecast.Enabled
	checks := []validationCheck{
		{value.AWS.Region != "us-east-1", "aws.region", "Cost Explorer requires us-east-1"},
		{value.AWS.RequestTimeout <= 0, "aws.request_timeout", "must be positive"},
		{value.AWS.Retry.MaxAttempts <= 0, "aws.retry.max_attempts", "must be positive"},
		{value.AWS.Retry.MaxAttempts > maxRetryAttempts, "aws.retry.max_attempts", "must not exceed 10"},
		{value.AWS.Retry.BaseDelay <= 0, "aws.retry.base_delay", "must be positive"},
		{value.AWS.Retry.MaxBackoff < value.AWS.Retry.BaseDelay, "aws.retry.max_backoff", "must not be less than base_delay"},
		{nonFinite(value.AWS.RateLimit.RequestsPerSecond), "aws.rate_limit.requests_per_second", "must be finite"},
		{value.AWS.RateLimit.RequestsPerSecond <= 0, "aws.rate_limit.requests_per_second", "must be positive"},
		{value.AWS.RateLimit.RequestsPerSecond > 1, "aws.rate_limit.requests_per_second", "must not exceed 1"},
		{value.AWS.RateLimit.Burst <= 0, "aws.rate_limit.burst", "must be positive"},
		{value.AWS.RateLimit.Burst > maxRateLimitBurst, "aws.rate_limit.burst", "must not exceed 5"},
		{value.CostExplorer.MaxPages <= 0, "cost_explorer.max_pages", "must be positive"},
		{value.CostExplorer.MaxPages > 200, "cost_explorer.max_pages", "must not exceed 200"},
		{value.CostExplorer.CostMetric != "UnblendedCost", "cost_explorer.cost_metric", "only UnblendedCost is supported"},
		{value.CostExplorer.RefreshInterval <= value.AWS.RequestTimeout, "cost_explorer.refresh_interval", "must exceed aws.request_timeout"},
		{nonFinite(value.CostExplorer.JitterRatio), "cost_explorer.jitter_ratio", "must be finite"},
		{value.CostExplorer.JitterRatio < 0 || value.CostExplorer.JitterRatio > 0.5, "cost_explorer.jitter_ratio", "must be between 0 and 0.5"},
		{value.CostExplorer.Forecast.Enabled && (value.CostExplorer.Forecast.PredictionInterval < 80 || value.CostExplorer.Forecast.PredictionInterval > 99), "cost_explorer.forecast.prediction_interval", "must be between 80 and 99"},
		{!anyCollector, "cost_explorer.collectors", "at least one collector must be enabled"},
		{value.CostExplorer.Dimensions.SeriesLimit <= 0, "cost_explorer.dimensions.series_limit", "must be positive"},
		{value.CostExplorer.Dimensions.SeriesLimit > 2000, "cost_explorer.dimensions.series_limit", "must not exceed 2000"},
		{value.CostExplorer.Dimensions.Overflow != "aggregate", "cost_explorer.dimensions.overflow", "only aggregate is supported"},
		{strings.TrimSpace(value.CostExplorer.Dimensions.OverflowLabel) == "", "cost_explorer.dimensions.overflow_label", "must not be empty"},
		{value.CostExplorer.Dimensions.OverflowLabel != strings.TrimSpace(value.CostExplorer.Dimensions.OverflowLabel), "cost_explorer.dimensions.overflow_label", "must not contain leading or trailing whitespace"},
		{value.Cache.FreshnessTTL < value.CostExplorer.RefreshInterval, "cache.freshness_ttl", "must not be less than refresh_interval"},
		{value.Cache.StaleAfter < value.Cache.FreshnessTTL, "cache.stale_after", "must not be less than freshness_ttl"},
		{value.Scheduler.MaxConcurrency <= 0, "scheduler.max_concurrency", "must be positive"},
		{value.Scheduler.FailureBackoff.Initial <= 0, "scheduler.failure_backoff.initial", "must be positive"},
		{value.Scheduler.FailureBackoff.Max < value.Scheduler.FailureBackoff.Initial, "scheduler.failure_backoff.max", "must not be less than initial"},
		{nonFinite(value.Scheduler.FailureBackoff.Multiplier), "scheduler.failure_backoff.multiplier", "must be finite"},
		{value.Scheduler.FailureBackoff.Multiplier <= 1, "scheduler.failure_backoff.multiplier", "must exceed 1"},
	}
	return firstValidationError(checks)
}

func firstValidationError(checks []validationCheck) error {
	for _, check := range checks {
		if check.invalid {
			return fmt.Errorf("%s: %s", check.path, check.message)
		}
	}

	return nil
}

func nonFinite(value float64) bool { return math.IsNaN(value) || math.IsInf(value, 0) }
