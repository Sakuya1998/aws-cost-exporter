// Package config defines the exporter's strongly typed configuration.
package config

import "time"

// Config is the complete application configuration.
type Config struct {
	Server       ServerConfig       `mapstructure:"server" yaml:"server"`
	Log          LogConfig          `mapstructure:"log" yaml:"log"`
	AWS          AWSConfig          `mapstructure:"aws" yaml:"aws"`
	CostExplorer CostExplorerConfig `mapstructure:"cost_explorer" yaml:"cost_explorer"`
	Cache        CacheConfig        `mapstructure:"cache" yaml:"cache"`
	Scheduler    SchedulerConfig    `mapstructure:"scheduler" yaml:"scheduler"`
	Telemetry    TelemetryConfig    `mapstructure:"telemetry" yaml:"telemetry"`
}

// ServerConfig controls the HTTP server.
type ServerConfig struct {
	ListenAddress     string        `mapstructure:"listen_address" yaml:"listen_address"`
	MetricsPath       string        `mapstructure:"metrics_path" yaml:"metrics_path"`
	ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout" yaml:"read_header_timeout"`
	ReadTimeout       time.Duration `mapstructure:"read_timeout" yaml:"read_timeout"`
	WriteTimeout      time.Duration `mapstructure:"write_timeout" yaml:"write_timeout"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout" yaml:"idle_timeout"`
	ShutdownTimeout   time.Duration `mapstructure:"shutdown_timeout" yaml:"shutdown_timeout"`
	MaxInFlight       int           `mapstructure:"max_in_flight" yaml:"max_in_flight"`
	Debug             DebugConfig   `mapstructure:"debug" yaml:"debug"`
}

// DebugConfig controls diagnostic HTTP endpoints.
type DebugConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
}

// LogConfig controls structured logging.
type LogConfig struct {
	Level     string `mapstructure:"level" yaml:"level"`
	Format    string `mapstructure:"format" yaml:"format"`
	AddSource bool   `mapstructure:"add_source" yaml:"add_source"`
}

// AWSConfig controls AWS SDK construction and request policy.
type AWSConfig struct {
	Region         string          `mapstructure:"region" yaml:"region"`
	Profile        string          `mapstructure:"profile" yaml:"profile"`
	EndpointURL    string          `mapstructure:"endpoint_url" yaml:"endpoint_url"`
	RequestTimeout time.Duration   `mapstructure:"request_timeout" yaml:"request_timeout"`
	Retry          RetryConfig     `mapstructure:"retry" yaml:"retry"`
	RateLimit      RateLimitConfig `mapstructure:"rate_limit" yaml:"rate_limit"`
}

// RetryConfig controls AWS SDK request retries.
type RetryConfig struct {
	MaxAttempts int           `mapstructure:"max_attempts" yaml:"max_attempts"`
	BaseDelay   time.Duration `mapstructure:"base_delay" yaml:"base_delay"`
	MaxBackoff  time.Duration `mapstructure:"max_backoff" yaml:"max_backoff"`
}

// RateLimitConfig controls the shared AWS request limiter.
type RateLimitConfig struct {
	RequestsPerSecond float64 `mapstructure:"requests_per_second" yaml:"requests_per_second"`
	Burst             int     `mapstructure:"burst" yaml:"burst"`
}

// CostExplorerConfig controls Cost Explorer collection.
type CostExplorerConfig struct {
	Enabled         bool             `mapstructure:"enabled" yaml:"enabled"`
	CostMetric      string           `mapstructure:"cost_metric" yaml:"cost_metric"`
	MaxPages        int              `mapstructure:"max_pages" yaml:"max_pages"`
	RefreshInterval time.Duration    `mapstructure:"refresh_interval" yaml:"refresh_interval"`
	StartupRefresh  bool             `mapstructure:"startup_refresh" yaml:"startup_refresh"`
	JitterRatio     float64          `mapstructure:"jitter_ratio" yaml:"jitter_ratio"`
	Forecast        ForecastConfig   `mapstructure:"forecast" yaml:"forecast"`
	Collectors      CollectorsConfig `mapstructure:"collectors" yaml:"collectors"`
	Dimensions      DimensionsConfig `mapstructure:"dimensions" yaml:"dimensions"`
	Filters         FiltersConfig    `mapstructure:"filters" yaml:"filters"`
}

// ForecastConfig controls AWS cost forecasting.
type ForecastConfig struct {
	Enabled            bool `mapstructure:"enabled" yaml:"enabled"`
	PredictionInterval int  `mapstructure:"prediction_interval" yaml:"prediction_interval"`
}

// CollectorsConfig enables built-in collector plugins.
type CollectorsConfig struct {
	Total   bool `mapstructure:"total" yaml:"total"`
	Service bool `mapstructure:"service" yaml:"service"`
	Region  bool `mapstructure:"region" yaml:"region"`
	Account bool `mapstructure:"account" yaml:"account"`
}

// DimensionsConfig controls grouped-series cardinality.
type DimensionsConfig struct {
	SeriesLimit   int    `mapstructure:"series_limit" yaml:"series_limit"`
	Overflow      string `mapstructure:"overflow" yaml:"overflow"`
	OverflowLabel string `mapstructure:"overflow_label" yaml:"overflow_label"`
}

// FiltersConfig restricts Cost Explorer query dimensions.
type FiltersConfig struct {
	LinkedAccountIDs []string `mapstructure:"linked_account_ids" yaml:"linked_account_ids"`
	Services         []string `mapstructure:"services" yaml:"services"`
	Regions          []string `mapstructure:"regions" yaml:"regions"`
}

// CacheConfig controls snapshot freshness state.
type CacheConfig struct {
	FreshnessTTL time.Duration `mapstructure:"freshness_ttl" yaml:"freshness_ttl"`
	StaleAfter   time.Duration `mapstructure:"stale_after" yaml:"stale_after"`
}

// SchedulerConfig controls collection concurrency and backoff.
type SchedulerConfig struct {
	MaxConcurrency int           `mapstructure:"max_concurrency" yaml:"max_concurrency"`
	FailureBackoff BackoffConfig `mapstructure:"failure_backoff" yaml:"failure_backoff"`
}

// BackoffConfig controls refresh-level exponential backoff.
type BackoffConfig struct {
	Initial    time.Duration `mapstructure:"initial" yaml:"initial"`
	Max        time.Duration `mapstructure:"max" yaml:"max"`
	Multiplier float64       `mapstructure:"multiplier" yaml:"multiplier"`
}

// TelemetryConfig controls standard Go runtime metrics.
type TelemetryConfig struct {
	IncludeGoCollector      bool `mapstructure:"include_go_collector" yaml:"include_go_collector"`
	IncludeProcessCollector bool `mapstructure:"include_process_collector" yaml:"include_process_collector"`
}

// Default returns an independent configuration populated with safe defaults.
func Default() Config {
	return Config{
		Server: ServerConfig{
			ListenAddress: ":8080", MetricsPath: "/metrics",
			ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
			WriteTimeout: 30 * time.Second, IdleTimeout: time.Minute,
			ShutdownTimeout: 15 * time.Second, MaxInFlight: 20,
			Debug: DebugConfig{},
		},
		Log: LogConfig{Level: "info", Format: "json"},
		AWS: AWSConfig{
			Region: "us-east-1", RequestTimeout: 30 * time.Second,
			Retry:     RetryConfig{MaxAttempts: 5, BaseDelay: time.Second, MaxBackoff: 30 * time.Second},
			RateLimit: RateLimitConfig{RequestsPerSecond: 0.5, Burst: 1},
		},
		CostExplorer: CostExplorerConfig{
			Enabled: true, CostMetric: "UnblendedCost", MaxPages: 50,
			RefreshInterval: 6 * time.Hour, StartupRefresh: true,
			JitterRatio: 0.10,
			Forecast:    ForecastConfig{Enabled: true, PredictionInterval: 80},
			Collectors:  CollectorsConfig{Total: true, Service: true, Region: true, Account: true},
			Dimensions:  DimensionsConfig{SeriesLimit: 1000, Overflow: "aggregate", OverflowLabel: "__other__"},
			Filters: FiltersConfig{
				LinkedAccountIDs: []string{}, Services: []string{}, Regions: []string{},
			},
		},
		Cache: CacheConfig{FreshnessTTL: 12 * time.Hour, StaleAfter: 24 * time.Hour},
		Scheduler: SchedulerConfig{
			MaxConcurrency: 2,
			FailureBackoff: BackoffConfig{
				Initial: time.Minute, Max: 30 * time.Minute, Multiplier: 2,
			},
		},
		Telemetry: TelemetryConfig{IncludeGoCollector: true, IncludeProcessCollector: true},
	}
}
