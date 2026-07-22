// Package config defines the exporter's strongly typed configuration.
package config

import "time"

const (
	CredentialSourceDefaultChain = "default_chain"
	CredentialSourceProfile      = "profile"
	CredentialSourceStaticEnv    = "static_env"
)

// Config is the complete v0.3 application configuration.
type Config struct {
	Server     ServerConfig     `mapstructure:"server" yaml:"server"`
	Log        LogConfig        `mapstructure:"log" yaml:"log"`
	AWS        AWSConfig        `mapstructure:"aws" yaml:"aws"`
	Targets    []TargetConfig   `mapstructure:"targets" yaml:"targets"`
	Collection CollectionConfig `mapstructure:"collection" yaml:"collection"`
	Cache      CacheConfig      `mapstructure:"cache" yaml:"cache"`
	Telemetry  TelemetryConfig  `mapstructure:"telemetry" yaml:"telemetry"`
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

// AWSConfig controls base AWS SDK construction and request policy.
type AWSConfig struct {
	Region         string            `mapstructure:"region" yaml:"region"`
	Credentials    CredentialsConfig `mapstructure:"credentials" yaml:"credentials"`
	RequestTimeout time.Duration     `mapstructure:"request_timeout" yaml:"request_timeout"`
	Endpoints      EndpointsConfig   `mapstructure:"endpoints" yaml:"endpoints"`
	Retry          RetryConfig       `mapstructure:"retry" yaml:"retry"`
	RateLimit      RateLimitConfig   `mapstructure:"rate_limit" yaml:"rate_limit"`
}

// CredentialsConfig contains named credential sources selected by targets.
type CredentialsConfig struct {
	Sources map[string]CredentialSourceConfig `mapstructure:"sources" yaml:"sources"`
}

// CredentialSourceConfig selects one AWS SDK credential provider chain.
// Static credentials are referenced by environment-variable name only.
type CredentialSourceConfig struct {
	Type               string `mapstructure:"type" yaml:"type"`
	Profile            string `mapstructure:"profile" yaml:"profile"`
	AccessKeyIDEnv     string `mapstructure:"access_key_id_env" yaml:"access_key_id_env"`
	SecretAccessKeyEnv string `mapstructure:"secret_access_key_env" yaml:"secret_access_key_env"`
	SessionTokenEnv    string `mapstructure:"session_token_env" yaml:"session_token_env"`
}

// EndpointsConfig provides service-specific endpoint overrides for testing.
type EndpointsConfig struct {
	STS           string `mapstructure:"sts" yaml:"sts"`
	CostExplorer  string `mapstructure:"cost_explorer" yaml:"cost_explorer"`
	Organizations string `mapstructure:"organizations" yaml:"organizations"`
	Budgets       string `mapstructure:"budgets" yaml:"budgets"`
	Athena        string `mapstructure:"athena" yaml:"athena"`
}

// RetryConfig controls AWS SDK request retries.
type RetryConfig struct {
	MaxAttempts int           `mapstructure:"max_attempts" yaml:"max_attempts"`
	BaseDelay   time.Duration `mapstructure:"base_delay" yaml:"base_delay"`
	MaxBackoff  time.Duration `mapstructure:"max_backoff" yaml:"max_backoff"`
}

// RateLimitConfig controls process-wide and per-target AWS attempt limiters.
type RateLimitConfig struct {
	GlobalRequestsPerSecond float64 `mapstructure:"global_requests_per_second" yaml:"global_requests_per_second"`
	GlobalBurst             int     `mapstructure:"global_burst" yaml:"global_burst"`
	TargetRequestsPerSecond float64 `mapstructure:"target_requests_per_second" yaml:"target_requests_per_second"`
	TargetBurst             int     `mapstructure:"target_burst" yaml:"target_burst"`
}

// TargetConfig is one explicitly configured AWS account boundary.
type TargetConfig struct {
	Name          string                    `mapstructure:"name" yaml:"name"`
	AccountID     string                    `mapstructure:"account_id" yaml:"account_id"`
	Required      bool                      `mapstructure:"required" yaml:"required"`
	Credentials   TargetCredentialsConfig   `mapstructure:"credentials" yaml:"credentials"`
	CostExplorer  TargetCostExplorerConfig  `mapstructure:"cost_explorer" yaml:"cost_explorer"`
	Organizations TargetOrganizationsConfig `mapstructure:"organizations" yaml:"organizations"`
	Budgets       TargetBudgetsConfig       `mapstructure:"budgets" yaml:"budgets"`
	Commitments   TargetCommitmentsConfig   `mapstructure:"commitments" yaml:"commitments"`
	Anomalies     TargetAnomaliesConfig     `mapstructure:"anomalies" yaml:"anomalies"`
	CUR           TargetCURConfig           `mapstructure:"cur" yaml:"cur"`
	Tags          TargetTagsConfig          `mapstructure:"tags" yaml:"tags"`
}

// TargetCredentialsConfig binds a target to a source and optional role.
type TargetCredentialsConfig struct {
	Source     string            `mapstructure:"source" yaml:"source"`
	AssumeRole *AssumeRoleConfig `mapstructure:"assume_role" yaml:"assume_role"`
}

// AssumeRoleConfig selects target credentials through STS.
type AssumeRoleConfig struct {
	RoleARN       string `mapstructure:"role_arn" yaml:"role_arn"`
	ExternalIDEnv string `mapstructure:"external_id_env" yaml:"external_id_env"`
	SessionName   string `mapstructure:"session_name" yaml:"session_name"`
}

// TargetCostExplorerConfig controls target-specific Cost Explorer filters.
type TargetCostExplorerConfig struct {
	Enabled bool          `mapstructure:"enabled" yaml:"enabled"`
	Filters FiltersConfig `mapstructure:"filters" yaml:"filters"`
}

// TargetOrganizationsConfig controls optional Organizations metadata.
type TargetOrganizationsConfig struct {
	Enabled    bool     `mapstructure:"enabled" yaml:"enabled"`
	AccountIDs []string `mapstructure:"account_ids" yaml:"account_ids"`
}

// TargetBudgetsConfig controls allowlisted AWS Budgets collection.
type TargetBudgetsConfig struct {
	Enabled bool     `mapstructure:"enabled" yaml:"enabled"`
	Names   []string `mapstructure:"names" yaml:"names"`
}

type TargetCommitmentsConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
}
type TargetAnomaliesConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
}

type TargetCURConfig struct {
	Enabled        bool           `mapstructure:"enabled" yaml:"enabled"`
	Database       string         `mapstructure:"database" yaml:"database"`
	Table          string         `mapstructure:"table" yaml:"table"`
	Workgroup      string         `mapstructure:"workgroup" yaml:"workgroup"`
	OutputLocation string         `mapstructure:"output_location" yaml:"output_location"`
	QueryTimeout   time.Duration  `mapstructure:"query_timeout" yaml:"query_timeout"`
	PollInterval   time.Duration  `mapstructure:"poll_interval" yaml:"poll_interval"`
	TagColumns     []CURTagColumn `mapstructure:"tag_columns" yaml:"tag_columns"`
}

type CURTagColumn struct {
	Key    string `mapstructure:"key" yaml:"key"`
	Column string `mapstructure:"column" yaml:"column"`
}

type TargetTagsConfig struct {
	Enabled bool           `mapstructure:"enabled" yaml:"enabled"`
	Keys    []TagKeyConfig `mapstructure:"keys" yaml:"keys"`
}

type TagKeyConfig struct {
	Key       string `mapstructure:"key" yaml:"key"`
	MaxValues int    `mapstructure:"max_values" yaml:"max_values"`
}

// FiltersConfig restricts Cost Explorer query dimensions.
type FiltersConfig struct {
	LinkedAccountIDs []string `mapstructure:"linked_account_ids" yaml:"linked_account_ids"`
	Services         []string `mapstructure:"services" yaml:"services"`
	Regions          []string `mapstructure:"regions" yaml:"regions"`
}

// CollectionConfig controls scheduler behavior and all collection domains.
type CollectionConfig struct {
	RefreshInterval time.Duration                 `mapstructure:"refresh_interval" yaml:"refresh_interval"`
	StartupRefresh  bool                          `mapstructure:"startup_refresh" yaml:"startup_refresh"`
	JitterRatio     float64                       `mapstructure:"jitter_ratio" yaml:"jitter_ratio"`
	MaxConcurrency  int                           `mapstructure:"max_concurrency" yaml:"max_concurrency"`
	FailureBackoff  BackoffConfig                 `mapstructure:"failure_backoff" yaml:"failure_backoff"`
	CostExplorer    CollectionCostExplorerConfig  `mapstructure:"cost_explorer" yaml:"cost_explorer"`
	Organizations   CollectionOrganizationsConfig `mapstructure:"organizations" yaml:"organizations"`
	Budgets         CollectionBudgetsConfig       `mapstructure:"budgets" yaml:"budgets"`
	Commitments     CollectionCommitmentsConfig   `mapstructure:"commitments" yaml:"commitments"`
	Anomalies       CollectionAnomaliesConfig     `mapstructure:"anomalies" yaml:"anomalies"`
	Tags            CollectionTagsConfig          `mapstructure:"tags" yaml:"tags"`
	CUR             CollectionCURConfig           `mapstructure:"cur" yaml:"cur"`
}

// CollectionCostExplorerConfig controls shared Cost Explorer semantics.
type CollectionCostExplorerConfig struct {
	CostBases          []string         `mapstructure:"cost_bases" yaml:"cost_bases"`
	MaxPages           int              `mapstructure:"max_pages" yaml:"max_pages"`
	PredictionInterval int              `mapstructure:"prediction_interval" yaml:"prediction_interval"`
	Collectors         CollectorsConfig `mapstructure:"collectors" yaml:"collectors"`
	Dimensions         DimensionsConfig `mapstructure:"dimensions" yaml:"dimensions"`
}

type CollectionCommitmentsConfig struct {
	RefreshInterval time.Duration `mapstructure:"refresh_interval" yaml:"refresh_interval"`
	MaxPages        int           `mapstructure:"max_pages" yaml:"max_pages"`
	SeriesLimit     int           `mapstructure:"series_limit" yaml:"series_limit"`
}

type CollectionAnomaliesConfig struct {
	RefreshInterval time.Duration `mapstructure:"refresh_interval" yaml:"refresh_interval"`
	MaxPages        int           `mapstructure:"max_pages" yaml:"max_pages"`
	SeriesLimit     int           `mapstructure:"series_limit" yaml:"series_limit"`
}

type CollectionTagsConfig struct {
	RefreshInterval time.Duration `mapstructure:"refresh_interval" yaml:"refresh_interval"`
	MaxPages        int           `mapstructure:"max_pages" yaml:"max_pages"`
	SeriesLimit     int           `mapstructure:"series_limit" yaml:"series_limit"`
}

type CollectionCURConfig struct {
	RefreshInterval time.Duration `mapstructure:"refresh_interval" yaml:"refresh_interval"`
	MaxPages        int           `mapstructure:"max_pages" yaml:"max_pages"`
	MaxRows         int           `mapstructure:"max_rows" yaml:"max_rows"`
	SeriesLimit     int           `mapstructure:"series_limit" yaml:"series_limit"`
}

// CollectorsConfig enables built-in Cost Explorer collector plugins.
type CollectorsConfig struct {
	Total    bool `mapstructure:"total" yaml:"total"`
	Service  bool `mapstructure:"service" yaml:"service"`
	Region   bool `mapstructure:"region" yaml:"region"`
	Account  bool `mapstructure:"account" yaml:"account"`
	Forecast bool `mapstructure:"forecast" yaml:"forecast"`
}

// DimensionsConfig controls grouped-series cardinality.
type DimensionsConfig struct {
	SeriesLimit   int    `mapstructure:"series_limit" yaml:"series_limit"`
	Overflow      string `mapstructure:"overflow" yaml:"overflow"`
	OverflowLabel string `mapstructure:"overflow_label" yaml:"overflow_label"`
}

// CollectionOrganizationsConfig controls Organizations refresh and bounds.
type CollectionOrganizationsConfig struct {
	RefreshInterval time.Duration `mapstructure:"refresh_interval" yaml:"refresh_interval"`
	MaxPages        int           `mapstructure:"max_pages" yaml:"max_pages"`
	SeriesLimit     int           `mapstructure:"series_limit" yaml:"series_limit"`
}

// CollectionBudgetsConfig controls Budgets refresh and bounds.
type CollectionBudgetsConfig struct {
	RefreshInterval time.Duration `mapstructure:"refresh_interval" yaml:"refresh_interval"`
	MaxPages        int           `mapstructure:"max_pages" yaml:"max_pages"`
	SeriesLimit     int           `mapstructure:"series_limit" yaml:"series_limit"`
}

// BackoffConfig controls refresh-level exponential backoff.
type BackoffConfig struct {
	MaxAttempts int           `mapstructure:"max_attempts" yaml:"max_attempts"`
	Initial     time.Duration `mapstructure:"initial" yaml:"initial"`
	Max         time.Duration `mapstructure:"max" yaml:"max"`
	Multiplier  float64       `mapstructure:"multiplier" yaml:"multiplier"`
}

// CacheConfig controls snapshot freshness state.
type CacheConfig struct {
	FreshnessTTL time.Duration `mapstructure:"freshness_ttl" yaml:"freshness_ttl"`
	StaleAfter   time.Duration `mapstructure:"stale_after" yaml:"stale_after"`
}

// TelemetryConfig controls standard Go runtime metrics.
type TelemetryConfig struct {
	IncludeGoCollector      bool `mapstructure:"include_go_collector" yaml:"include_go_collector"`
	IncludeProcessCollector bool `mapstructure:"include_process_collector" yaml:"include_process_collector"`
}

// Default returns an independent configuration populated with safe defaults.
// Targets intentionally have no default because account identity must be explicit.
func Default() Config {
	return Config{
		Server: ServerConfig{
			ListenAddress: ":8080", MetricsPath: "/metrics",
			ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
			WriteTimeout: 30 * time.Second, IdleTimeout: time.Minute,
			ShutdownTimeout: 15 * time.Second, MaxInFlight: 20,
		},
		Log: LogConfig{Level: "info", Format: "json"},
		AWS: AWSConfig{
			Region: "us-east-1", RequestTimeout: 30 * time.Second,
			Credentials: CredentialsConfig{Sources: map[string]CredentialSourceConfig{}},
			Retry:       RetryConfig{MaxAttempts: 5, BaseDelay: time.Second, MaxBackoff: 30 * time.Second},
			RateLimit: RateLimitConfig{
				GlobalRequestsPerSecond: 1, GlobalBurst: 2,
				TargetRequestsPerSecond: 0.5, TargetBurst: 1,
			},
		},
		Targets: []TargetConfig{},
		Collection: CollectionConfig{
			RefreshInterval: 6 * time.Hour, StartupRefresh: true, JitterRatio: 0.10,
			MaxConcurrency: 4,
			FailureBackoff: BackoffConfig{MaxAttempts: 3, Initial: time.Minute, Max: 30 * time.Minute, Multiplier: 2},
			CostExplorer: CollectionCostExplorerConfig{
				CostBases: []string{"unblended"}, MaxPages: 50, PredictionInterval: 80,
				Collectors: CollectorsConfig{Total: true, Service: true, Region: true, Account: true, Forecast: true},
				Dimensions: DimensionsConfig{SeriesLimit: 1000, Overflow: "aggregate", OverflowLabel: "__other__"},
			},
			Organizations: CollectionOrganizationsConfig{RefreshInterval: 24 * time.Hour, MaxPages: 20, SeriesLimit: 1000},
			Budgets:       CollectionBudgetsConfig{RefreshInterval: 6 * time.Hour, MaxPages: 20, SeriesLimit: 100},
			Commitments:   CollectionCommitmentsConfig{RefreshInterval: 24 * time.Hour, MaxPages: 20, SeriesLimit: 20},
			Anomalies:     CollectionAnomaliesConfig{RefreshInterval: 6 * time.Hour, MaxPages: 20, SeriesLimit: 10},
			Tags:          CollectionTagsConfig{RefreshInterval: 6 * time.Hour, MaxPages: 50, SeriesLimit: 500},
			CUR:           CollectionCURConfig{RefreshInterval: 24 * time.Hour, MaxPages: 50, MaxRows: 5000, SeriesLimit: 2000},
		},
		Cache:     CacheConfig{FreshnessTTL: 12 * time.Hour, StaleAfter: 24 * time.Hour},
		Telemetry: TelemetryConfig{IncludeGoCollector: true, IncludeProcessCollector: true},
	}
}
