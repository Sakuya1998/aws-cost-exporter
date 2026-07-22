package config

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
)

const (
	maxTargets        = 20
	maxRetryAttempts  = 10
	maxRateLimitBurst = 5
	maxRequestsPerSec = 10
	maxPages          = 200
	maxCostSeries     = 2000
	maxOrgSeries      = 2000
	maxBudgetSeries   = 500
	maxTagKeys        = 20
	maxTagValues      = 500
)

var (
	targetNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	accountIDPattern   = regexp.MustCompile(`^[0-9]{12}$`)
	roleARNPattern     = regexp.MustCompile(`^arn:(aws|aws-us-gov|aws-cn):iam::([0-9]{12}):role/[A-Za-z0-9+=,.@_/-]{1,512}$`)
	environmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	sessionNamePattern = regexp.MustCompile(`^[A-Za-z0-9+=,.@_-]{2,64}$`)
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

// Validate checks all v0.3 field and cross-field invariants. Environment
// references are resolved here so --check-config and production startup agree.
func Validate(value Config) error {
	if err := ValidateServer(value.Server); err != nil {
		return err
	}
	if err := validateBase(value); err != nil {
		return err
	}
	if err := validateCredentialSources(value.AWS.Credentials); err != nil {
		return err
	}
	return validateTargets(value)
}

func validateBase(value Config) error {
	collectors := value.Collection.CostExplorer.Collectors
	anyCollector := collectors.Total || collectors.Service || collectors.Region || collectors.Account || collectors.Forecast
	checks := []validationCheck{
		{value.Log.Level != "debug" && value.Log.Level != "info" && value.Log.Level != "warn" && value.Log.Level != "error", "log.level", "must be one of debug, info, warn, error"},
		{value.Log.Format != "json" && value.Log.Format != "text", "log.format", "must be json or text"},
		{value.AWS.Region != "us-east-1", "aws.region", "billing APIs require us-east-1"},
		{value.AWS.RequestTimeout <= 0, "aws.request_timeout", "must be positive"},
		{value.AWS.Retry.MaxAttempts <= 0 || value.AWS.Retry.MaxAttempts > maxRetryAttempts, "aws.retry.max_attempts", "must be between 1 and 10"},
		{value.AWS.Retry.BaseDelay <= 0, "aws.retry.base_delay", "must be positive"},
		{value.AWS.Retry.MaxBackoff < value.AWS.Retry.BaseDelay, "aws.retry.max_backoff", "must not be less than base_delay"},
		{invalidRate(value.AWS.RateLimit.GlobalRequestsPerSecond), "aws.rate_limit.global_requests_per_second", "must be finite and between 0 and 10"},
		{value.AWS.RateLimit.GlobalBurst <= 0 || value.AWS.RateLimit.GlobalBurst > maxRateLimitBurst, "aws.rate_limit.global_burst", "must be between 1 and 5"},
		{invalidRate(value.AWS.RateLimit.TargetRequestsPerSecond), "aws.rate_limit.target_requests_per_second", "must be finite and between 0 and 10"},
		{value.AWS.RateLimit.TargetBurst <= 0 || value.AWS.RateLimit.TargetBurst > maxRateLimitBurst, "aws.rate_limit.target_burst", "must be between 1 and 5"},
		{len(value.Targets) == 0 || len(value.Targets) > maxTargets, "targets", "must contain between 1 and 20 entries"},
		{value.Collection.RefreshInterval <= value.AWS.RequestTimeout, "collection.refresh_interval", "must exceed aws.request_timeout"},
		{nonFinite(value.Collection.JitterRatio), "collection.jitter_ratio", "must be finite"},
		{value.Collection.JitterRatio < 0 || value.Collection.JitterRatio > 0.5, "collection.jitter_ratio", "must be between 0 and 0.5"},
		{value.Collection.MaxConcurrency <= 0, "collection.max_concurrency", "must be positive"},
		{value.Collection.FailureBackoff.MaxAttempts <= 0 || value.Collection.FailureBackoff.MaxAttempts > maxRetryAttempts, "collection.failure_backoff.max_attempts", "must be between 1 and 10"},
		{value.Collection.FailureBackoff.Initial <= 0, "collection.failure_backoff.initial", "must be positive"},
		{value.Collection.FailureBackoff.Max < value.Collection.FailureBackoff.Initial, "collection.failure_backoff.max", "must not be less than initial"},
		{nonFinite(value.Collection.FailureBackoff.Multiplier), "collection.failure_backoff.multiplier", "must be finite"},
		{value.Collection.FailureBackoff.Multiplier <= 1, "collection.failure_backoff.multiplier", "must exceed 1"},
		{!anyCollector, "collection.cost_explorer.collectors", "at least one collector must be enabled"},
		{len(value.Collection.CostExplorer.CostBases) == 0, "collection.cost_explorer.cost_bases", "must contain at least one basis"},
		{value.Collection.CostExplorer.MaxPages <= 0 || value.Collection.CostExplorer.MaxPages > maxPages, "collection.cost_explorer.max_pages", "must be between 1 and 200"},
		{collectors.Forecast && (value.Collection.CostExplorer.PredictionInterval < 80 || value.Collection.CostExplorer.PredictionInterval > 99), "collection.cost_explorer.prediction_interval", "must be between 80 and 99"},
		{value.Collection.CostExplorer.Dimensions.SeriesLimit <= 0 || value.Collection.CostExplorer.Dimensions.SeriesLimit > maxCostSeries, "collection.cost_explorer.dimensions.series_limit", "must be between 1 and 2000"},
		{value.Collection.CostExplorer.Dimensions.Overflow != "aggregate", "collection.cost_explorer.dimensions.overflow", "only aggregate is supported"},
		{strings.TrimSpace(value.Collection.CostExplorer.Dimensions.OverflowLabel) == "", "collection.cost_explorer.dimensions.overflow_label", "must not be empty"},
		{value.Collection.CostExplorer.Dimensions.OverflowLabel != strings.TrimSpace(value.Collection.CostExplorer.Dimensions.OverflowLabel), "collection.cost_explorer.dimensions.overflow_label", "must not contain leading or trailing whitespace"},
		{value.Collection.Organizations.RefreshInterval <= value.AWS.RequestTimeout, "collection.organizations.refresh_interval", "must exceed aws.request_timeout"},
		{value.Collection.Organizations.MaxPages <= 0 || value.Collection.Organizations.MaxPages > maxPages, "collection.organizations.max_pages", "must be between 1 and 200"},
		{value.Collection.Organizations.SeriesLimit <= 0 || value.Collection.Organizations.SeriesLimit > maxOrgSeries, "collection.organizations.series_limit", "must be between 1 and 2000"},
		{value.Collection.Budgets.RefreshInterval <= value.AWS.RequestTimeout, "collection.budgets.refresh_interval", "must exceed aws.request_timeout"},
		{value.Collection.Budgets.MaxPages <= 0 || value.Collection.Budgets.MaxPages > maxPages, "collection.budgets.max_pages", "must be between 1 and 200"},
		{value.Collection.Budgets.SeriesLimit <= 0 || value.Collection.Budgets.SeriesLimit > maxBudgetSeries, "collection.budgets.series_limit", "must be between 1 and 500"},
		{value.Collection.Commitments.RefreshInterval <= value.AWS.RequestTimeout, "collection.commitments.refresh_interval", "must exceed aws.request_timeout"},
		{value.Collection.Commitments.MaxPages <= 0 || value.Collection.Commitments.MaxPages > maxPages, "collection.commitments.max_pages", "must be between 1 and 200"},
		{value.Collection.Commitments.SeriesLimit <= 0 || value.Collection.Commitments.SeriesLimit > maxCostSeries, "collection.commitments.series_limit", "must be between 1 and 2000"},
		{value.Collection.Anomalies.RefreshInterval <= value.AWS.RequestTimeout, "collection.anomalies.refresh_interval", "must exceed aws.request_timeout"},
		{value.Collection.Anomalies.MaxPages <= 0 || value.Collection.Anomalies.MaxPages > maxPages, "collection.anomalies.max_pages", "must be between 1 and 200"},
		{value.Collection.Anomalies.SeriesLimit <= 0 || value.Collection.Anomalies.SeriesLimit > 20, "collection.anomalies.series_limit", "must be between 1 and 20"},
		{value.Collection.Tags.MaxPages <= 0 || value.Collection.Tags.MaxPages > maxPages, "collection.tags.max_pages", "must be between 1 and 200"},
		{value.Collection.Tags.RefreshInterval <= value.AWS.RequestTimeout, "collection.tags.refresh_interval", "must exceed aws.request_timeout"},
		{value.Collection.Tags.SeriesLimit <= 0 || value.Collection.Tags.SeriesLimit > maxBudgetSeries, "collection.tags.series_limit", "must be between 1 and 500"},
		{value.Collection.CUR.RefreshInterval <= value.AWS.RequestTimeout, "collection.cur.refresh_interval", "must exceed aws.request_timeout"},
		{value.Collection.CUR.MaxPages <= 0 || value.Collection.CUR.MaxPages > maxPages, "collection.cur.max_pages", "must be between 1 and 200"},
		{value.Collection.CUR.MaxRows <= 0 || value.Collection.CUR.MaxRows > 100000, "collection.cur.max_rows", "must be between 1 and 100000"},
		{value.Collection.CUR.SeriesLimit <= 0 || value.Collection.CUR.SeriesLimit > 20000, "collection.cur.series_limit", "must be between 1 and 20000"},
		{value.Cache.FreshnessTTL < value.Collection.RefreshInterval, "cache.freshness_ttl", "must not be less than collection.refresh_interval"},
		{value.Cache.StaleAfter < value.Cache.FreshnessTTL, "cache.stale_after", "must not be less than freshness_ttl"},
	}
	return firstValidationError(checks)
}

func validateTargets(value Config) error {
	names := make(map[string]struct{}, len(value.Targets))
	accounts := make(map[string]struct{}, len(value.Targets))
	roles := make(map[string]struct{}, len(value.Targets))
	directSources := make(map[string]struct{}, len(value.Targets))
	requiredCostTargets := 0
	for index, target := range value.Targets {
		path := fmt.Sprintf("targets[%d]", index)
		if !targetNamePattern.MatchString(target.Name) {
			return fmt.Errorf("%s.name: must match %s", path, targetNamePattern)
		}
		if _, duplicate := names[target.Name]; duplicate {
			return fmt.Errorf("%s.name: must be unique", path)
		}
		names[target.Name] = struct{}{}
		if !accountIDPattern.MatchString(target.AccountID) {
			return fmt.Errorf("%s.account_id: must contain 12 digits", path)
		}
		if _, duplicate := accounts[target.AccountID]; duplicate {
			return fmt.Errorf("%s.account_id: must be unique", path)
		}
		accounts[target.AccountID] = struct{}{}
		if !targetNamePattern.MatchString(target.Credentials.Source) {
			return fmt.Errorf("%s.credentials.source: must reference a valid credential source", path)
		}
		if _, exists := value.AWS.Credentials.Sources[target.Credentials.Source]; !exists {
			return fmt.Errorf("%s.credentials.source: credential source does not exist", path)
		}
		if !target.CostExplorer.Enabled && !target.Organizations.Enabled && !target.Budgets.Enabled && !target.Commitments.Enabled && !target.Anomalies.Enabled && !target.CUR.Enabled && !target.Tags.Enabled {
			return fmt.Errorf("%s: at least one integration must be enabled", path)
		}
		if target.Required {
			if !target.CostExplorer.Enabled {
				return fmt.Errorf("%s.required: requires cost_explorer.enabled", path)
			}
			requiredCostTargets++
		}
		if err := validateAssumeRole(path+".credentials", target, roles); err != nil {
			return err
		}
		if target.Credentials.AssumeRole == nil {
			if _, duplicate := directSources[target.Credentials.Source]; duplicate {
				return fmt.Errorf("%s.credentials.source: a credential source may be used by at most one direct target", path)
			}
			directSources[target.Credentials.Source] = struct{}{}
		}
		if err := validateFilters(path+".cost_explorer.filters", target.CostExplorer.Filters); err != nil {
			return err
		}
		if err := validateOrganizations(path, target, value); err != nil {
			return err
		}
		if err := validateBudgets(path, target, value.Collection.Budgets.SeriesLimit); err != nil {
			return err
		}
		if err := validateCommitments(path, target); err != nil {
			return err
		}
		if err := validateAnomalies(path, target); err != nil {
			return err
		}
		if err := validateCUR(path, target); err != nil {
			return err
		}
		if err := validateTags(path, target, value.Collection.Tags.SeriesLimit, len(value.Collection.CostExplorer.CostBases)); err != nil {
			return err
		}
	}
	for index, basis := range value.Collection.CostExplorer.CostBases {
		if basis != "unblended" && basis != "amortized" && basis != "net" {
			return fmt.Errorf("collection.cost_explorer.cost_bases[%d]: must be unblended, amortized, or net", index)
		}
	}
	if err := validateUniqueStrings("collection.cost_explorer.cost_bases", value.Collection.CostExplorer.CostBases, nil); err != nil {
		return err
	}
	if requiredCostTargets == 0 {
		return fmt.Errorf("targets: at least one required Cost Explorer target is required")
	}
	return nil
}

func validateCommitments(path string, target TargetConfig) error {
	if !target.Commitments.Enabled {
		return nil
	}
	return nil
}

func validateAnomalies(path string, target TargetConfig) error {
	if !target.Anomalies.Enabled {
		return nil
	}
	return nil
}

var sqlIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
var workgroupPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
var s3LocationPattern = regexp.MustCompile(`^s3://[A-Za-z0-9][A-Za-z0-9.-]{1,61}[A-Za-z0-9]/.+$`)

func validateCUR(path string, target TargetConfig) error {
	if !target.CUR.Enabled {
		return nil
	}
	checks := []struct{ value, field string }{
		{target.CUR.Database, "database"}, {target.CUR.Table, "table"}, {target.CUR.Workgroup, "workgroup"}, {target.CUR.OutputLocation, "output_location"},
	}
	for _, check := range checks {
		if strings.TrimSpace(check.value) == "" || check.value != strings.TrimSpace(check.value) {
			return fmt.Errorf("%s.cur.%s: must be non-empty without surrounding whitespace", path, check.field)
		}
	}
	if !sqlIdentifierPattern.MatchString(target.CUR.Database) {
		return fmt.Errorf("%s.cur.database: must be a safe SQL identifier", path)
	}
	if !sqlIdentifierPattern.MatchString(target.CUR.Table) {
		return fmt.Errorf("%s.cur.table: must be a safe SQL identifier", path)
	}
	if !workgroupPattern.MatchString(target.CUR.Workgroup) {
		return fmt.Errorf("%s.cur.workgroup: has invalid format", path)
	}
	if !s3LocationPattern.MatchString(target.CUR.OutputLocation) {
		return fmt.Errorf("%s.cur.output_location: must be an s3:// prefix", path)
	}
	if target.CUR.QueryTimeout <= 0 {
		return fmt.Errorf("%s.cur.query_timeout: must be positive", path)
	}
	if target.CUR.PollInterval <= 0 {
		return fmt.Errorf("%s.cur.poll_interval: must be positive", path)
	}
	if target.CUR.PollInterval >= target.CUR.QueryTimeout {
		return fmt.Errorf("%s.cur.poll_interval: must be less than query_timeout", path)
	}
	if len(target.CUR.TagColumns) > maxTagKeys {
		return fmt.Errorf("%s.cur.tag_columns: must contain at most %d entries", path, maxTagKeys)
	}
	seen := map[string]struct{}{}
	for index, column := range target.CUR.TagColumns {
		if column.Key == "" || column.Key != strings.TrimSpace(column.Key) {
			return fmt.Errorf("%s.cur.tag_columns[%d].key: invalid", path, index)
		}
		if !sqlIdentifierPattern.MatchString(column.Column) {
			return fmt.Errorf("%s.cur.tag_columns[%d].column: must be a safe SQL identifier", path, index)
		}
		if _, exists := seen[column.Key]; exists {
			return fmt.Errorf("%s.cur.tag_columns[%d].key: must be unique", path, index)
		}
		seen[column.Key] = struct{}{}
	}
	return nil
}

func validateTags(path string, target TargetConfig, seriesLimit, basisCount int) error {
	if !target.Tags.Enabled {
		return nil
	}
	if !target.CUR.Enabled && !target.CostExplorer.Enabled {
		return fmt.Errorf("%s.tags.enabled: requires cur or cost_explorer", path)
	}
	if len(target.Tags.Keys) == 0 || len(target.Tags.Keys) > maxTagKeys {
		return fmt.Errorf("%s.tags.keys: must contain between 1 and %d entries", path, maxTagKeys)
	}
	seen := map[string]struct{}{}
	columns := map[string]struct{}{}
	for _, item := range target.CUR.TagColumns {
		columns[item.Key] = struct{}{}
	}
	for index, key := range target.Tags.Keys {
		if key.Key == "" || key.Key != strings.TrimSpace(key.Key) {
			return fmt.Errorf("%s.tags.keys[%d].key: invalid", path, index)
		}
		if key.MaxValues <= 0 || key.MaxValues > maxTagValues {
			return fmt.Errorf("%s.tags.keys[%d].max_values: must be between 1 and %d", path, index, maxTagValues)
		}
		if _, exists := seen[key.Key]; exists {
			return fmt.Errorf("%s.tags.keys[%d].key: must be unique", path, index)
		}
		seen[key.Key] = struct{}{}
		if target.CUR.Enabled {
			if _, exists := columns[key.Key]; !exists {
				return fmt.Errorf("%s.tags.keys[%d].key: requires a matching cur.tag_columns entry", path, index)
			}
		}
	}
	if target.CUR.Enabled && len(columns) != len(seen) {
		return fmt.Errorf("%s.cur.tag_columns: must exactly match tags.keys", path)
	}
	if len(target.Tags.Keys)*2*max(1, basisCount) > seriesLimit {
		return fmt.Errorf("%s.tags.keys: exceeds collection.tags.series_limit", path)
	}
	return nil
}

func validateAssumeRole(path string, target TargetConfig, roles map[string]struct{}) error {
	if target.Credentials.AssumeRole == nil {
		return nil
	}
	role := target.Credentials.AssumeRole
	match := roleARNPattern.FindStringSubmatch(role.RoleARN)
	if len(match) != 3 || strings.Contains(role.RoleARN, "*") {
		return fmt.Errorf("%s.assume_role.role_arn: must be an exact IAM role ARN without wildcards", path)
	}
	if match[2] != target.AccountID {
		return fmt.Errorf("%s.assume_role.role_arn: account must match account_id", path)
	}
	if _, duplicate := roles[role.RoleARN]; duplicate {
		return fmt.Errorf("%s.assume_role.role_arn: must be unique", path)
	}
	roles[role.RoleARN] = struct{}{}
	if !environmentPattern.MatchString(role.ExternalIDEnv) {
		return fmt.Errorf("%s.assume_role.external_id_env: must be a valid environment variable name", path)
	}
	if externalID, exists := os.LookupEnv(role.ExternalIDEnv); !exists || strings.TrimSpace(externalID) == "" {
		return fmt.Errorf("%s.assume_role.external_id_env: referenced environment variable must be set and non-empty", path)
	}
	if role.SessionName != "" && !sessionNamePattern.MatchString(role.SessionName) {
		return fmt.Errorf("%s.assume_role.session_name: must be 2..64 AWS STS session characters", path)
	}
	return nil
}

func validateCredentialSources(value CredentialsConfig) error {
	if len(value.Sources) == 0 || len(value.Sources) > maxTargets {
		return fmt.Errorf("aws.credentials.sources: must contain between 1 and 20 entries")
	}
	for name, source := range value.Sources {
		path := "aws.credentials.sources." + name
		if !targetNamePattern.MatchString(name) {
			return fmt.Errorf("%s: source name must match %s", path, targetNamePattern)
		}
		switch source.Type {
		case CredentialSourceDefaultChain:
			if source.Profile != "" || source.AccessKeyIDEnv != "" || source.SecretAccessKeyEnv != "" || source.SessionTokenEnv != "" {
				return fmt.Errorf("%s: default_chain does not accept profile or static environment fields", path)
			}
		case CredentialSourceProfile:
			if source.Profile == "" || source.Profile != strings.TrimSpace(source.Profile) {
				return fmt.Errorf("%s.profile: must be non-empty without surrounding whitespace", path)
			}
			if source.AccessKeyIDEnv != "" || source.SecretAccessKeyEnv != "" || source.SessionTokenEnv != "" {
				return fmt.Errorf("%s: profile does not accept static environment fields", path)
			}
		case CredentialSourceStaticEnv:
			if source.Profile != "" {
				return fmt.Errorf("%s.profile: static_env does not accept profile", path)
			}
			if err := validateSecretEnvironment(path+".access_key_id_env", source.AccessKeyIDEnv, true); err != nil {
				return err
			}
			if err := validateSecretEnvironment(path+".secret_access_key_env", source.SecretAccessKeyEnv, true); err != nil {
				return err
			}
			if err := validateSecretEnvironment(path+".session_token_env", source.SessionTokenEnv, false); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s.type: must be one of default_chain, profile, static_env", path)
		}
	}
	return nil
}

func validateSecretEnvironment(path, name string, required bool) error {
	if name == "" && !required {
		return nil
	}
	if !environmentPattern.MatchString(name) {
		return fmt.Errorf("%s: must be a valid environment variable name", path)
	}
	if secret, exists := os.LookupEnv(name); !exists || strings.TrimSpace(secret) == "" {
		return fmt.Errorf("%s: referenced environment variable must be set and non-empty", path)
	}
	return nil
}

func validateOrganizations(path string, target TargetConfig, value Config) error {
	if !target.Organizations.Enabled {
		return nil
	}
	ids := target.Organizations.AccountIDs
	if len(ids) == 0 {
		if !target.CostExplorer.Enabled || !value.Collection.CostExplorer.Collectors.Account {
			return fmt.Errorf("%s.organizations.account_ids: observed mode requires the account cost collector", path)
		}
		if value.Collection.Organizations.SeriesLimit < value.Collection.CostExplorer.Dimensions.SeriesLimit {
			return fmt.Errorf("collection.organizations.series_limit: must cover the account collector series limit in observed mode")
		}
		return nil
	}
	if len(ids) > value.Collection.Organizations.SeriesLimit {
		return fmt.Errorf("%s.organizations.account_ids: exceeds collection.organizations.series_limit", path)
	}
	return validateUniqueStrings(path+".organizations.account_ids", ids, accountIDPattern)
}

func validateBudgets(path string, target TargetConfig, limit int) error {
	if !target.Budgets.Enabled {
		return nil
	}
	if len(target.Budgets.Names) == 0 {
		return fmt.Errorf("%s.budgets.names: must not be empty when budgets is enabled", path)
	}
	if len(target.Budgets.Names)*3 > limit {
		return fmt.Errorf("%s.budgets.names: exceeds collection.budgets.series_limit", path)
	}
	return validateUniqueStrings(path+".budgets.names", target.Budgets.Names, nil)
}

func validateFilters(path string, filters FiltersConfig) error {
	if err := validateUniqueStrings(path+".linked_account_ids", filters.LinkedAccountIDs, accountIDPattern); err != nil {
		return err
	}
	if err := validateUniqueStrings(path+".services", filters.Services, nil); err != nil {
		return err
	}
	return validateUniqueStrings(path+".regions", filters.Regions, nil)
}

func validateUniqueStrings(path string, values []string, pattern *regexp.Regexp) error {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if value == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("%s[%d]: must be non-empty without surrounding whitespace", path, index)
		}
		if pattern != nil && !pattern.MatchString(value) {
			return fmt.Errorf("%s[%d]: has invalid format", path, index)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s[%d]: must be unique", path, index)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func invalidRate(value float64) bool {
	return nonFinite(value) || value <= 0 || value > maxRequestsPerSec
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
