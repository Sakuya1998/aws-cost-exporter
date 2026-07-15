package docs_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type policy struct {
	Version   string      `json:"Version"`
	Statement []statement `json:"Statement"`
}

type statement struct {
	Effect   string      `json:"Effect"`
	Action   interface{} `json:"Action"`
	Resource interface{} `json:"Resource"`
}

func TestREADMEContainsDeploymentAndRuntimeContracts(t *testing.T) {
	content := read(t, filepath.Join("..", "..", "README.md"))
	for _, fragment := range []string{
		"github.com/sakuya1998/aws-cost-exporter",
		"ghcr.io/sakuya1998/aws-cost-exporter",
		"oci://ghcr.io/sakuya1998/charts/aws-cost-exporter",
		"make build", "docker compose up --build", "helm install",
		"--config", "--listen-address", "--log-level", "--check-config", "--version",
		"AWS_COST_EXPORTER_SERVER__LISTEN_ADDRESS",
		"/metrics", "/healthz", "/ready", "/version",
		"ce:GetCostAndUsage", "ce:GetCostForecast",
		"Cost Explorer API requests are billed", "not a financial reconciliation system",
		"Never sum different `currency`", "does not call AWS during a Prometheus scrape",
		"max_pages", "series_limit", "8 `GetCostAndUsage`",
		"AWSCostExplorerPaginationSpike", "AWSCostExplorerThrottleSustained",
		"pagination_pages_total", "Apache License", "LICENSE",
		"configs/aws-cost-exporter.example.yaml", "dashboards/grafana/aws-cost-exporter.json",
		"rules/prometheus/aws-cost-exporter.rules.yaml", "docs/operations/troubleshooting.md",
	} {
		if !strings.Contains(content, fragment) {
			t.Errorf("README lacks %q", fragment)
		}
	}
}

func TestREADMETracksProductionMetricNames(t *testing.T) {
	readme := read(t, filepath.Join("..", "..", "README.md"))
	sources := []struct {
		name, prefix, pattern string
	}{
		{"cost.go", "aws_cost_", `newDesc\("([^"]+)"`},
		{"exporter.go", "aws_cost_exporter_", `(?:selfDesc|counter|histogram)\("([^"]+)"`},
	}
	for _, source := range sources {
		content := read(t, filepath.Join("..", "..", "internal", "metrics", source.name))
		for _, match := range regexp.MustCompile(source.pattern).FindAllStringSubmatch(content, -1) {
			metric := source.prefix + match[1]
			if !strings.Contains(readme, metric) {
				t.Errorf("README lacks production metric %q", metric)
			}
		}
	}
}

func TestTroubleshootingCoversOperationalFailureModes(t *testing.T) {
	content := read(t, filepath.Join("..", "..", "docs", "operations", "troubleshooting.md"))
	for _, fragment := range []string{
		"/ready", "`missing`", "`stale`", "collector_up", "cache_age_seconds",
		"403", "Cost Explorer", "throttling", "aws_api_requests_total",
		"pagination_pages_total", "SDK retries",
		"`__other__`", "dimension_overflow_values_total", "backfill",
		"currency", "today through month end", "replica", "debug",
	} {
		if !strings.Contains(content, fragment) {
			t.Errorf("troubleshooting guide lacks %q", fragment)
		}
	}
}

func TestIAMExamplesAreValidAndLeastPrivilege(t *testing.T) {
	directory := filepath.Join("..", "..", "examples", "iam")
	files := []string{
		"mvp-readonly.json", "organizations-readonly.json",
		"assume-role-trust.json", "assume-role-permissions.json",
	}
	for _, name := range files {
		var document any
		if err := json.Unmarshal([]byte(read(t, filepath.Join(directory, name))), &document); err != nil {
			t.Errorf("%s is invalid JSON: %v", name, err)
		}
	}
	var mvp policy
	if err := json.Unmarshal([]byte(read(t, filepath.Join(directory, "mvp-readonly.json"))), &mvp); err != nil {
		t.Fatal(err)
	}
	if mvp.Version != "2012-10-17" || len(mvp.Statement) != 1 {
		t.Fatalf("unexpected MVP policy structure: %+v", mvp)
	}
	encoded, _ := json.Marshal(mvp)
	text := string(encoded)
	for _, action := range []string{"ce:GetCostAndUsage", "ce:GetCostForecast"} {
		if !strings.Contains(text, action) {
			t.Errorf("MVP policy lacks %s", action)
		}
	}
	for _, forbidden := range []string{"GetDimensionValues", "organizations:", "sts:", "access_key", "secret"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("MVP policy contains forbidden capability %q", forbidden)
		}
	}
	permissions := read(t, filepath.Join(directory, "assume-role-permissions.json"))
	if !strings.Contains(permissions, "sts:AssumeRole") ||
		strings.Contains(permissions, `"Resource": "*"`) {
		t.Error("AssumeRole permission must target one explicit role ARN")
	}
	trust := read(t, filepath.Join(directory, "assume-role-trust.json"))
	if !strings.Contains(trust, "sts:ExternalId") || !strings.Contains(trust, `"AWS"`) {
		t.Error("trust example must identify a principal and require ExternalId")
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
