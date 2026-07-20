package config_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
)

func TestValidateCredentialSources(t *testing.T) {
	t.Setenv("STATIC_ACCESS", "access")
	t.Setenv("STATIC_SECRET", "secret")
	base := validConfig()
	base.AWS.Credentials.Sources["workstation"] = config.CredentialSourceConfig{Type: config.CredentialSourceProfile, Profile: "account-a"}
	base.AWS.Credentials.Sources["legacy"] = config.CredentialSourceConfig{
		Type: config.CredentialSourceStaticEnv, AccessKeyIDEnv: "STATIC_ACCESS", SecretAccessKeyEnv: "STATIC_SECRET",
	}
	if err := config.Validate(base); err != nil {
		t.Fatalf("valid credential sources = %v", err)
	}
	for _, test := range []struct {
		name, path string
		mutate     func(*config.Config)
	}{
		{"missing sources", "aws.credentials.sources", func(v *config.Config) { v.AWS.Credentials.Sources = nil }},
		{"unknown type", ".type", func(v *config.Config) {
			v.AWS.Credentials.Sources["runtime"] = config.CredentialSourceConfig{Type: "keys"}
		}},
		{"profile whitespace", ".profile", func(v *config.Config) {
			v.AWS.Credentials.Sources["runtime"] = config.CredentialSourceConfig{Type: config.CredentialSourceProfile, Profile: " account-a "}
		}},
		{"default fields", "default_chain", func(v *config.Config) {
			v.AWS.Credentials.Sources["runtime"] = config.CredentialSourceConfig{Type: config.CredentialSourceDefaultChain, Profile: "default"}
		}},
		{"missing static env", "access_key_id_env", func(v *config.Config) {
			v.AWS.Credentials.Sources["runtime"] = config.CredentialSourceConfig{Type: config.CredentialSourceStaticEnv, AccessKeyIDEnv: "UNSET_STATIC_ACCESS", SecretAccessKeyEnv: "STATIC_SECRET"}
		}},
		{"unknown target source", "credentials.source", func(v *config.Config) { v.Targets[0].Credentials.Source = "missing" }},
		{"duplicate account", "account_id", func(v *config.Config) {
			copy := v.Targets[0]
			copy.Name = "other"
			copy.Credentials.Source = "workstation"
			v.Targets = append(v.Targets, copy)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := base
			value.AWS.Credentials.Sources = make(map[string]config.CredentialSourceConfig, len(base.AWS.Credentials.Sources))
			for name, source := range base.AWS.Credentials.Sources {
				value.AWS.Credentials.Sources[name] = source
			}
			value.Targets = append([]config.TargetConfig(nil), base.Targets...)
			test.mutate(&value)
			err := config.Validate(value)
			if err == nil || !strings.Contains(err.Error(), test.path) {
				t.Fatalf("Validate()=%v, want %q", err, test.path)
			}
		})
	}
}

func TestValidateEnforcesV02Invariants(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, path string
		mutate     func(*config.Config)
	}{
		{"targets missing", "targets", func(v *config.Config) { v.Targets = nil }},
		{"target name", "targets[0].name", func(v *config.Config) { v.Targets[0].Name = "Bad Name" }},
		{"account id", "targets[0].account_id", func(v *config.Config) { v.Targets[0].AccountID = "123" }},
		{"required integration", "targets[0].required", func(v *config.Config) {
			v.Targets[0].CostExplorer.Enabled = false
			v.Targets[0].Budgets.Enabled = true
			v.Targets[0].Budgets.Names = []string{"Monthly"}
		}},
		{"metrics path", "server.metrics_path", func(v *config.Config) { v.Server.MetricsPath = "/ready" }},
		{"global rate finite", "aws.rate_limit.global_requests_per_second", func(v *config.Config) { v.AWS.RateLimit.GlobalRequestsPerSecond = math.NaN() }},
		{"target rate finite", "aws.rate_limit.target_requests_per_second", func(v *config.Config) { v.AWS.RateLimit.TargetRequestsPerSecond = math.Inf(1) }},
		{"global burst", "aws.rate_limit.global_burst", func(v *config.Config) { v.AWS.RateLimit.GlobalBurst = 6 }},
		{"retry attempts", "aws.retry.max_attempts", func(v *config.Config) { v.AWS.Retry.MaxAttempts = 11 }},
		{"refresh", "collection.refresh_interval", func(v *config.Config) { v.Collection.RefreshInterval = v.AWS.RequestTimeout }},
		{"collector retry attempts", "collection.failure_backoff.max_attempts", func(v *config.Config) { v.Collection.FailureBackoff.MaxAttempts = 11 }},
		{"jitter", "collection.jitter_ratio", func(v *config.Config) { v.Collection.JitterRatio = math.NaN() }},
		{"backoff", "collection.failure_backoff.multiplier", func(v *config.Config) { v.Collection.FailureBackoff.Multiplier = math.Inf(1) }},
		{"overflow whitespace", "collection.cost_explorer.dimensions.overflow_label", func(v *config.Config) { v.Collection.CostExplorer.Dimensions.OverflowLabel = " __other__ " }},
		{"budgets names", "targets[0].budgets.names", func(v *config.Config) { v.Targets[0].Budgets.Enabled = true }},
		{"organizations observed", "organizations.account_ids", func(v *config.Config) {
			v.Targets[0].Organizations.Enabled = true
			v.Collection.CostExplorer.Collectors.Account = false
		}},
		{"freshness", "cache.freshness_ttl", func(v *config.Config) { v.Cache.FreshnessTTL = time.Hour }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			value := validConfig()
			test.mutate(&value)
			err := config.Validate(value)
			if err == nil || !strings.Contains(err.Error(), test.path) {
				t.Fatalf("Validate() = %v, want %q", err, test.path)
			}
		})
	}
}

func TestValidateAssumeRoleAndTargetUniqueness(t *testing.T) {
	t.Setenv("TARGET_EXTERNAL_ID", "private")
	base := validConfig()
	base.Targets[0].Credentials.AssumeRole = &config.AssumeRoleConfig{RoleARN: "arn:aws:iam::444455556666:role/exporter", ExternalIDEnv: "TARGET_EXTERNAL_ID"}
	if err := config.Validate(base); err != nil {
		t.Fatalf("valid role = %v", err)
	}
	for _, test := range []struct {
		name, path string
		mutate     func(*config.Config)
	}{
		{"wildcard", "role_arn", func(v *config.Config) {
			v.Targets[0].Credentials.AssumeRole.RoleARN = "arn:aws:iam::444455556666:role/*"
		}},
		{"account mismatch", "role_arn", func(v *config.Config) {
			v.Targets[0].Credentials.AssumeRole.RoleARN = "arn:aws:iam::111122223333:role/exporter"
		}},
		{"duplicate target", "name", func(v *config.Config) { v.Targets = append(v.Targets, v.Targets[0]) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := base
			value.Targets = append([]config.TargetConfig(nil), base.Targets...)
			role := *base.Targets[0].Credentials.AssumeRole
			value.Targets[0].Credentials.AssumeRole = &role
			test.mutate(&value)
			err := config.Validate(value)
			if err == nil || !strings.Contains(err.Error(), test.path) {
				t.Fatalf("Validate()=%v", err)
			}
		})
	}
}

func TestValidateDirectAndOptionalTargets(t *testing.T) {
	value := validConfig()
	value.Targets = append(value.Targets, config.TargetConfig{Name: "optional-budget", AccountID: "111122223333", Credentials: config.TargetCredentialsConfig{Source: "runtime", AssumeRole: &config.AssumeRoleConfig{RoleARN: "arn:aws:iam::111122223333:role/exporter", ExternalIDEnv: "OPTIONAL_EXTERNAL_ID"}}, Budgets: config.TargetBudgetsConfig{Enabled: true, Names: []string{"Monthly"}}})
	t.Setenv("OPTIONAL_EXTERNAL_ID", "private")
	if err := config.Validate(value); err != nil {
		t.Fatalf("Validate(optional target)=%v", err)
	}
	value.Targets[1].Credentials.AssumeRole = nil
	if err := config.Validate(value); err == nil || !strings.Contains(err.Error(), "direct target") {
		t.Fatalf("Validate(two direct)=%v", err)
	}
}
