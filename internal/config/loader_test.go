package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
)

// TestLoadAppliesDocumentedPrecedence verifies overrides beat environment,
// environment beats YAML, and omitted fields retain defaults.
func TestLoadAppliesDocumentedPrecedence(t *testing.T) {
	t.Setenv("AWS_COST_EXPORTER_LOG__LEVEL", "debug")
	path := filepath.Join(t.TempDir(), "config.yaml")
	document := []byte("server:\n  listen_address: \":9200\"\nlog:\n  level: warn\naws:\n  request_timeout: 45s\n")
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := config.Load(config.Options{
		Path: path,
		Overrides: map[string]any{
			"server.listen_address": ":9400",
		},
	})
	if err != nil {
		t.Fatalf("Load() returned an unexpected error: %v", err)
	}
	if got.Server.ListenAddress != ":9400" || got.Log.Level != "debug" {
		t.Fatalf("Load() precedence result = address %q, level %q", got.Server.ListenAddress, got.Log.Level)
	}
	if got.AWS.RequestTimeout != 45*time.Second || got.Server.MetricsPath != "/metrics" {
		t.Fatalf("Load() failed duration or default decoding: %#v", got)
	}
	if err := config.Check(config.Options{Path: path}); err != nil {
		t.Fatalf("Check() returned an unexpected error: %v", err)
	}
}

// TestLoadRejectsUnknownFieldsWithoutLeakingValues verifies strict decoding
// reports the field path but not its potentially sensitive value.
func TestLoadRejectsUnknownFieldsWithoutLeakingValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	document := []byte("aws:\n  secret_access_key: super-secret-value\n")
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := config.Load(config.Options{Path: path})
	if err == nil || !strings.Contains(err.Error(), "secret_access_key") {
		t.Fatalf("Load() error = %v, want unknown field path", err)
	}
	if strings.Contains(err.Error(), "super-secret-value") {
		t.Fatalf("Load() leaked configuration value: %v", err)
	}
}
