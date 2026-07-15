package app

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

// TestExecuteHandlesVersionCheckAndOverrides verifies every CLI-only path.
func TestExecuteHandlesVersionCheckAndOverrides(t *testing.T) {
	var output, errors bytes.Buffer
	calls := 0
	runtime := func(_ context.Context, value config.Config, _ *slog.Logger) error {
		calls++
		if value.Server.ListenAddress != ":9999" || value.Log.Level != "debug" {
			t.Fatalf("runtime config = %#v", value)
		}
		return nil
	}
	for _, test := range []struct {
		arg, want string
	}{{"--version", version.Current().String() + "\n"}, {"--check-config", "configuration valid\n"}} {
		output.Reset()
		if err := execute(context.Background(), []string{test.arg}, &output, &errors, runtime); err != nil ||
			output.String() != test.want || calls != 0 {
			t.Fatalf("%s output=%q calls=%d error=%v", test.arg, output.String(), calls, err)
		}
	}
	if err := execute(context.Background(), []string{
		"--listen-address=:9999", "--log-level=debug",
	}, &output, &errors, runtime); err != nil || calls != 1 {
		t.Fatalf("runtime calls=%d error=%v", calls, err)
	}
	err := execute(
		context.Background(), []string{"--log-level=private-value", "--check-config"},
		&output, &output, func(context.Context, config.Config, *slog.Logger) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "log.level") ||
		strings.Contains(err.Error(), "AWS_SECRET_ACCESS_KEY") {
		t.Fatalf("execute(invalid) error = %v", err)
	}
	t.Setenv("AWS_COST_EXPORTER_COST_EXPLORER__ENABLED", "false")
	err = execute(context.Background(), []string{"--check-config"}, &output, &errors, runtime)
	if err == nil || !strings.Contains(err.Error(), "no collectors enabled") {
		t.Fatalf("execute(disabled) error = %v", err)
	}
}

func TestEnabledCollectorsHonorsFeatureFlags(t *testing.T) {
	value := config.Default()
	if got, want := enabledCollectors(value), []string{"total", "service", "region", "account", "forecast"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("enabledCollectors(default) = %v, want %v", got, want)
	}
	value.CostExplorer.Enabled = false
	if got := enabledCollectors(value); got != nil {
		t.Fatalf("enabledCollectors(disabled) = %v, want nil", got)
	}
}

func TestUnfilteredGroupedCollectorsDetectsMissingFilters(t *testing.T) {
	value := config.Default()
	if !unfilteredGroupedCollectors(value) {
		t.Fatal("default grouped collectors without filters should warn")
	}
	value.CostExplorer.Filters.Services = []string{"Amazon EC2"}
	if unfilteredGroupedCollectors(value) {
		t.Fatal("filters.services should suppress unfiltered warning")
	}
}
