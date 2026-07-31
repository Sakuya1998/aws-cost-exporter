package docs_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

type policy struct {
	Version   string      `json:"Version"`
	Statement []statement `json:"Statement"`
}

type statement struct {
	Sid       string                            `json:"Sid"`
	Effect    string                            `json:"Effect"`
	Action    interface{}                       `json:"Action"`
	Resource  interface{}                       `json:"Resource"`
	Condition map[string]map[string]interface{} `json:"Condition"`
}

func TestREADMEIsStableLandingPage(t *testing.T) {
	content := read(t, filepath.Join("..", "..", "README.md"))
	for _, fragment := range []string{
		"github.com/sakuya1998/aws-cost-exporter",
		"v0.3.0",
		"ghcr.io/sakuya1998/aws-cost-exporter",
		"oci://ghcr.io/sakuya1998/charts/aws-cost-exporter",
		"make build", "docker compose up --build", "helm install",
		"/metrics", "/healthz", "/ready", "/version",
		"not a financial reconciliation system", "does not call AWS during a Prometheus scrape",
		"https://github.com/Sakuya1998/aws-cost-exporter/wiki",
		"Home-zh-CN", "Apache License", "LICENSE",
	} {
		if !strings.Contains(content, fragment) {
			t.Errorf("README lacks %q", fragment)
		}
	}
}

func TestV1EntryPointsRemainInProgressUntilReleaseEvidenceExists(t *testing.T) {
	readme := read(t, filepath.Join("..", "..", "README.md"))
	for _, fragment := range []string{"v1.0 stabilization is in progress", "docs/operations/v1-slo.md", "docs/releases/v1.0-checklist.md"} {
		if !strings.Contains(readme, fragment) {
			t.Errorf("README lacks v1 entry point %q", fragment)
		}
	}
	roadmap := read(t, filepath.Join("..", "..", "ROADMAP.md"))
	for _, fragment := range []string{"## v1.0: Stable operational contract", "Status: in progress", "v1.0-checklist.md", "24-hour stability", "real AWS", "upgrade and rollback"} {
		if !strings.Contains(roadmap, fragment) {
			t.Errorf("ROADMAP lacks v1 status %q", fragment)
		}
	}
	if strings.Contains(roadmap, "Status: completed in v1.0.0") {
		t.Error("ROADMAP marks v1.0 complete before release evidence exists")
	}
	adr := read(t, filepath.Join("..", "..", "docs", "adr", "0002-ha-refresh-coordination.md"))
	for _, fragment := range []string{"reaffirmed for v1.0", "maxSurge: 0", "maxUnavailable: 1", "cost-first", "temporary metrics outage"} {
		if !strings.Contains(adr, fragment) {
			t.Errorf("ADR 0002 lacks v1 decision %q", fragment)
		}
	}
}

func TestTroubleshootingCoversOperationalFailureModes(t *testing.T) {
	content := read(t, filepath.Join("..", "..", "docs", "wiki", "Troubleshooting-and-Logging.md"))
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
		if _, duplicate := statements[item.Sid]; duplicate {
			t.Fatalf("CUR policy contains duplicate Sid %s", item.Sid)
		}
		statements[item.Sid] = item
	}

	assertStatement := func(sid string, actions, resources []string) statement {
		t.Helper()
		item, ok := statements[sid]
		if !ok {
			t.Fatalf("CUR policy lacks %s statement", sid)
		}
		assertExactStrings(t, sid+" actions", item.Action, actions)
		assertExactStrings(t, sid+" resources", item.Resource, resources)
		return item
	}
	assertStatement("ReadCURCatalog",
		[]string{"glue:GetDatabase", "glue:GetTable", "glue:GetPartitions"},
		[]string{"arn:aws:glue:us-east-1:444455556666:catalog", "arn:aws:glue:us-east-1:444455556666:database/billing", "arn:aws:glue:us-east-1:444455556666:table/billing/cur2"})
	assertStatement("LocateCURBuckets", []string{"s3:GetBucketLocation"},
		[]string{"arn:aws:s3:::example-cur-bucket", "arn:aws:s3:::example-athena-results"})
	inputList := assertStatement("ListCURInput", []string{"s3:ListBucket"}, []string{"arn:aws:s3:::example-cur-bucket"})
	assertPrefixCondition(t, inputList, []string{"cur-prefix", "cur-prefix/*"})
	outputList := assertStatement("ListAthenaResults", []string{"s3:ListBucket"}, []string{"arn:aws:s3:::example-athena-results"})
	assertPrefixCondition(t, outputList, []string{"aws-cost-exporter", "aws-cost-exporter/*"})
	assertStatement("ListAthenaResultUploads", []string{"s3:ListBucketMultipartUploads"}, []string{"arn:aws:s3:::example-athena-results"})
	assertStatement("ReadCURObjects", []string{"s3:GetObject"}, []string{"arn:aws:s3:::example-cur-bucket/cur-prefix/*"})
	assertStatement("AccessAthenaResults",
		[]string{"s3:GetObject", "s3:PutObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts"},
		[]string{"arn:aws:s3:::example-athena-results/aws-cost-exporter/*"})

	for _, item := range document.Statement {
		for _, resource := range stringValues(t, item.Resource) {
			if resource == "*" {
				t.Errorf("CUR policy %s uses wildcard resources", item.Sid)
			}
		}
	}
}

func assertExactStrings(t *testing.T, name string, value interface{}, expected []string) {
	t.Helper()
	actual := stringValues(t, value)
	slices.Sort(actual)
	expected = append([]string(nil), expected...)
	slices.Sort(expected)
	if !slices.Equal(actual, expected) {
		t.Errorf("%s=%v want %v", name, actual, expected)
	}
}

func stringValues(t *testing.T, value interface{}) []string {
	t.Helper()
	switch current := value.(type) {
	case string:
		return []string{current}
	case []interface{}:
		result := make([]string, len(current))
		for index, item := range current {
			text, ok := item.(string)
			if !ok {
				t.Fatalf("policy value %v is not a string", item)
			}
			result[index] = text
		}
		return result
	default:
		t.Fatalf("unsupported policy value type %T", value)
		return nil
	}
}

func assertPrefixCondition(t *testing.T, item statement, expected []string) {
	t.Helper()
	condition, ok := item.Condition["StringLike"]
	if !ok {
		t.Fatalf("CUR policy %s lacks StringLike condition", item.Sid)
	}
	value, ok := condition["s3:prefix"]
	if !ok {
		t.Fatalf("CUR policy %s lacks s3:prefix condition", item.Sid)
	}
	assertExactStrings(t, item.Sid+" prefixes", value, expected)
}

func TestCURDocumentationRequiresMatchingAthenaRegion(t *testing.T) {
	content := read(t, filepath.Join("..", "..", "docs", "wiki", "CUR-and-Athena.md"))
	if !strings.Contains(content, "Athena workgroup ARN region must match `targets[].cur.region`") {
		t.Error("CUR Wiki does not bind the Athena workgroup ARN region to targets[].cur.region")
	}
}

func TestV1SLODocumentsReferenceCapacityAndGrowthEvaluation(t *testing.T) {
	content := read(t, filepath.Join("..", "..", "docs", "operations", "v1-slo.md"))
	for _, fragment := range []string{
		"20 targets", "20,000", "2 vCPU", "512 MiB", "p99 latency below 5 seconds", "99.9%",
		"24 hours", "every 15 seconds", "AWS or Athena", "least-squares slopes", "second half",
		"4 MiB/hour heap", "8 MiB/hour RSS", "must not be cited as v1.0.0 acceptance",
	} {
		if !strings.Contains(content, fragment) {
			t.Errorf("v1 SLO documentation lacks %q", fragment)
		}
	}
}

func TestV1ThreatModelAndSecurityResponseAreComplete(t *testing.T) {
	threatModel := read(t, filepath.Join("..", "..", "docs", "security", "threat-model.md"))
	for _, fragment := range []string{
		"Assets", "Trust boundaries", "endpoint override", "signed requests", "configuration write access",
		"/metrics", "financial telemetry", "Spoofing", "Tampering", "Repudiation", "Information disclosure",
		"Denial of service", "Elevation of privilege", "Mitigations", "Residual risks", "Non-goals",
	} {
		if !strings.Contains(threatModel, fragment) {
			t.Errorf("threat model lacks %q", fragment)
		}
	}
	security := read(t, filepath.Join("..", "..", "SECURITY.md"))
	for _, fragment := range []string{
		"v1.x", "three business days", "seven business days", "coordinated disclosure",
		"revoke", "replace", "release artifacts",
	} {
		if !strings.Contains(security, fragment) {
			t.Errorf("SECURITY.md lacks %q", fragment)
		}
	}
}

func TestRepositoryTextContainsNoCredentialMaterial(t *testing.T) {
	root := filepath.Join("..", "..")
	patterns := map[string]*regexp.Regexp{
		"AWS access key ID": regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		"private key":       regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----`),
		"shared credential": regexp.MustCompile(`(?im)^\s*aws_secret_access_key\s*=\s*[^\s#]+`),
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".worktrees", "dist", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.IndexByte(content, 0) >= 0 {
			return nil
		}
		for name, pattern := range patterns {
			if pattern.Match(content) {
				t.Errorf("%s contains apparent %s", path, name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIAMWildcardResourcesAreLimitedToUnsupportedResourceScoping(t *testing.T) {
	directory := filepath.Join("..", "..", "examples", "iam")
	allowedWildcard := map[string]bool{
		"mvp-readonly.json": true, "organizations-readonly.json": true, "commitments-anomalies-readonly.json": true,
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var document policy
		if err := json.Unmarshal([]byte(read(t, filepath.Join(directory, entry.Name()))), &document); err != nil {
			t.Fatal(err)
		}
		for _, item := range document.Statement {
			if item.Resource == nil {
				continue
			}
			for _, resource := range stringValues(t, item.Resource) {
				if resource == "*" && !allowedWildcard[entry.Name()] {
					t.Errorf("%s %s uses avoidable wildcard resource", entry.Name(), item.Sid)
				}
			}
		}
	}
	guidance := read(t, filepath.Join(directory, "README.md"))
	for _, fragment := range []string{"Cost Explorer", "Organizations", "Resource: *", "does not support resource-level permissions", "sts:AssumeRole"} {
		if !strings.Contains(guidance, fragment) {
			t.Errorf("IAM guidance lacks %q", fragment)
		}
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
