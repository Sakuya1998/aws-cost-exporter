package app

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

func writeTestConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	document := []byte("targets:\n  - name: payer-prod\n    account_id: \"444455556666\"\n    required: true\n    cost_explorer:\n      enabled: true\n")
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExecuteHandlesVersionCheckAndOverrides(t *testing.T) {
	path := writeTestConfig(t)
	var output, errorOutput bytes.Buffer
	calls := 0
	runtime := func(_ context.Context, value config.Config, _ *slog.Logger) error {
		calls++
		if value.Server.ListenAddress != ":9999" || value.Log.Level != "debug" {
			t.Fatalf("runtime config=%#v", value)
		}
		return nil
	}
	if err := execute(context.Background(), []string{"--version"}, &output, &errorOutput, runtime); err != nil || output.String() != version.Current().String()+"\n" {
		t.Fatalf("version=%q,%v", output.String(), err)
	}
	output.Reset()
	if err := execute(context.Background(), []string{"--config", path, "--check-config"}, &output, &errorOutput, runtime); err != nil || output.String() != "configuration valid\n" || calls != 0 {
		t.Fatalf("check=%q calls=%d err=%v", output.String(), calls, err)
	}
	if err := execute(context.Background(), []string{"--config", path, "--listen-address=:9999", "--log-level=debug"}, &output, &errorOutput, runtime); err != nil || calls != 1 {
		t.Fatalf("runtime calls=%d err=%v", calls, err)
	}
	err := execute(context.Background(), []string{"--config", path, "--log-level=private", "--check-config"}, &output, &errorOutput, runtime)
	if err == nil || !strings.Contains(err.Error(), "log.level") {
		t.Fatalf("invalid log=%v", err)
	}
}

func TestConfiguredCollectorIDsAndRequiredTargets(t *testing.T) {
	value := config.Default()
	value.Targets = []config.TargetConfig{{Name: "payer", AccountID: "444455556666", Required: true, CostExplorer: config.TargetCostExplorerConfig{Enabled: true}}, {Name: "optional", AccountID: "111122223333", Organizations: config.TargetOrganizationsConfig{Enabled: true, AccountIDs: []string{"111122223333"}}, AssumeRole: &config.AssumeRoleConfig{RoleARN: "arn:aws:iam::111122223333:role/exporter", ExternalIDEnv: "IGNORED_IN_THIS_HELPER"}}}
	got := configuredCollectorIDs(value)
	want := []identity.CollectorID{{Target: "payer", Name: "total"}, {Target: "payer", Name: "service"}, {Target: "payer", Name: "region"}, {Target: "payer", Name: "account"}, {Target: "payer", Name: "forecast"}, {Target: "optional", Name: "organizations"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("configuredCollectorIDs=%v,want %v", got, want)
	}
}

func TestUnfilteredGroupedCollectorsUsesPerTargetFilters(t *testing.T) {
	value := config.Default()
	target := config.TargetConfig{Name: "payer", CostExplorer: config.TargetCostExplorerConfig{Enabled: true}}
	if !unfilteredGroupedCollectors(value, target) {
		t.Fatal("expected warning")
	}
	target.CostExplorer.Filters.Services = []string{"Amazon EC2"}
	if unfilteredGroupedCollectors(value, target) {
		t.Fatal("target filter should suppress warning")
	}
}

func TestCheckConfigRejectsRuntimeInvalidServerConfig(t *testing.T) {
	t.Setenv("AWS_COST_EXPORTER_SERVER__WRITE_TIMEOUT", "0s")
	var output, errorOutput bytes.Buffer
	err := execute(context.Background(), []string{"--config", writeTestConfig(t), "--check-config"}, &output, &errorOutput, func(context.Context, config.Config, *slog.Logger) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "server.write_timeout") || strings.Contains(output.String(), "configuration valid") {
		t.Fatalf("output=%q err=%v", output.String(), err)
	}
}
