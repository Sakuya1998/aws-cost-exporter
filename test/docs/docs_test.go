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
	Sid      string      `json:"Sid"`
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
		"cost_bases", "max_currencies", "overflow_label", "shutdown_timeout",
		"scheduler_shutdown_timeouts_total",
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
		{"cost.go", "aws_cost_", `costDesc\("([^"]+)"`},
		{"cost.go", "aws_budget_", `budgetDesc\("([^"]+)"`},
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
		"scheduler_shutdown_timeouts_total", "overflow_label",
		"currency", "max_currencies", "today through month end", "replica", "debug",
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
		"budgets-readonly.json", "commitments-anomalies-readonly.json", "cur-athena-readonly.json",
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
	if budgets := read(t, filepath.Join(directory, "budgets-readonly.json")); !strings.Contains(budgets, "budgets:ViewBudget") {
		t.Error("Budgets example lacks budgets:ViewBudget")
	}
	assertCURPolicy(t, filepath.Join(directory, "cur-athena-readonly.json"))
}

func assertCURPolicy(t *testing.T, path string) {
	t.Helper()
	var document policy
	if err := json.Unmarshal([]byte(read(t, path)), &document); err != nil {
		t.Fatal(err)
	}
	statements := make(map[string]statement, len(document.Statement))
	for _, item := range document.Statement {
		statements[item.Sid] = item
	}

	assertStatement := func(sid string, actions, resources []string) {
		t.Helper()
		item, ok := statements[sid]
		if !ok {
			t.Errorf("CUR policy lacks %s statement", sid)
			return
		}
		encoded, _ := json.Marshal(item)
		text := string(encoded)
		for _, value := range append(actions, resources...) {
			if !strings.Contains(text, value) {
				t.Errorf("CUR policy %s lacks %s", sid, value)
			}
		}
	}
	assertStatement("AccessCURBuckets",
		[]string{"s3:GetBucketLocation", "s3:ListBucket", "s3:ListBucketMultipartUploads"},
		[]string{"arn:aws:s3:::example-cur-bucket", "arn:aws:s3:::example-athena-results"})
	assertStatement("ReadCURObjects", []string{"s3:GetObject"}, []string{"arn:aws:s3:::example-cur-bucket/cur-prefix/*"})
	assertStatement("AccessAthenaResults",
		[]string{"s3:GetObject", "s3:PutObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts"},
		[]string{"arn:aws:s3:::example-athena-results/aws-cost-exporter/*"})

	for _, sid := range []string{"ReadCURObjects", "AccessAthenaResults"} {
		encoded, _ := json.Marshal(statements[sid])
		if strings.Contains(string(encoded), "s3:GetBucketLocation") || strings.Contains(string(encoded), "s3:ListBucket") {
			t.Errorf("CUR policy %s applies bucket actions to object ARNs", sid)
		}
	}
}

func TestCURDocumentationRequiresMatchingAthenaRegion(t *testing.T) {
	content := read(t, filepath.Join("..", "..", "README.md"))
	if !strings.Contains(content, "Athena workgroup ARN region must match `targets[].cur.region`") {
		t.Error("README does not bind the Athena workgroup ARN region to targets[].cur.region")
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
