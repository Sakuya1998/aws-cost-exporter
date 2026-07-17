package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
)

const minimalCredentialsYAML = "aws:\n  credentials:\n    sources:\n      runtime:\n        type: default_chain\n"
const minimalTargetOnlyYAML = "targets:\n  - name: payer-prod\n    account_id: \"444455556666\"\n    required: true\n    credentials:\n      source: runtime\n    cost_explorer:\n      enabled: true\n"
const minimalTargetYAML = minimalCredentialsYAML + minimalTargetOnlyYAML

func TestLoadAppliesDocumentedPrecedence(t *testing.T) {
	t.Setenv("AWS_COST_EXPORTER_LOG__LEVEL", "debug")
	path := filepath.Join(t.TempDir(), "config.yaml")
	document := []byte("server:\n  listen_address: \":9200\"\nlog:\n  level: warn\naws:\n  request_timeout: 45s\n  credentials:\n    sources:\n      runtime:\n        type: default_chain\n" + minimalTargetOnlyYAML)
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(config.Options{Path: path, Overrides: map[string]any{"server.listen_address": ":9400"}})
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if got.Server.ListenAddress != ":9400" || got.Log.Level != "debug" || got.AWS.RequestTimeout != 45*time.Second {
		t.Fatalf("precedence result = %#v", got)
	}
	if err := config.Check(config.Options{Path: path}); err != nil {
		t.Fatalf("Check() = %v", err)
	}
}

func TestLoadRejectsUnknownAndV01FieldsWithoutLeakingValues(t *testing.T) {
	for _, document := range []string{
		"aws:\n  secret_access_key: super-secret-value\n  credentials:\n    sources:\n      runtime:\n        type: default_chain\n" + minimalTargetOnlyYAML,
		"aws:\n  profile: legacy\n  credentials:\n    sources:\n      runtime:\n        type: default_chain\n" + minimalTargetOnlyYAML,
		minimalCredentialsYAML + "targets:\n  - name: payer-prod\n    account_id: \"444455556666\"\n    required: true\n    credentials:\n      source: runtime\n    assume_role:\n      role_arn: arn:aws:iam::444455556666:role/legacy\n    cost_explorer:\n      enabled: true\n",
		"cost_explorer:\n  enabled: true\n" + minimalTargetYAML,
		"scheduler:\n  max_concurrency: 2\n" + minimalTargetYAML,
	} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := config.Load(config.Options{Path: path})
		if err == nil {
			t.Fatalf("Load() accepted unknown document %q", document)
		}
		if strings.Contains(err.Error(), "super-secret-value") {
			t.Fatalf("Load() leaked value: %v", err)
		}
	}
}

func TestLoadResolvesExternalIDEnvironmentDuringCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	document := []byte("aws:\n  credentials:\n    sources:\n      runtime:\n        type: default_chain\ntargets:\n  - name: payer-prod\n    account_id: \"444455556666\"\n    required: true\n    credentials:\n      source: runtime\n      assume_role:\n        role_arn: arn:aws:iam::444455556666:role/aws-cost-exporter-reader\n        external_id_env: TEST_EXTERNAL_ID\n    cost_explorer:\n      enabled: true\n")
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Check(config.Options{Path: path}); err == nil || !strings.Contains(err.Error(), "external_id_env") {
		t.Fatalf("Check(unset env) = %v", err)
	}
	t.Setenv("TEST_EXTERNAL_ID", "private-value")
	if err := config.Check(config.Options{Path: path}); err != nil {
		t.Fatalf("Check(set env) = %v", err)
	}
}
