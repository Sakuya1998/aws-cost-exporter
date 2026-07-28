package docs_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var wikiPages = []string{
	"Home",
	"Getting-Started",
	"Installation",
	"Configuration-Reference",
	"Credentials-and-Targets",
	"Cost-Explorer",
	"CUR-and-Athena",
	"Optional-Collectors",
	"Metrics-Reference",
	"Dashboards-PromQL-and-Alerts",
	"Operations-and-Cost-Control",
	"Troubleshooting-and-Logging",
	"IAM-and-Security",
	"Architecture",
	"Development-and-Testing",
	"Releases-and-Supply-Chain",
}

func TestWikiPageManifestAndLanguagePairs(t *testing.T) {
	directory := filepath.Join("..", "..", "docs", "wiki")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read wiki directory: %v", err)
	}

	expected := []string{"README.md", "_Footer.md", "_Sidebar.md"}
	for _, page := range wikiPages {
		expected = append(expected, page+".md", page+"-zh-CN.md")
	}
	slices.Sort(expected)

	var actual []string
	for _, entry := range entries {
		if !entry.IsDir() {
			actual = append(actual, entry.Name())
		}
	}
	slices.Sort(actual)
	if !slices.Equal(actual, expected) {
		t.Fatalf("wiki files=%v want %v", actual, expected)
	}
}

func TestWikiPagesHaveLanguageSwitches(t *testing.T) {
	for _, page := range wikiPages {
		english := read(t, wikiFile(page+".md"))
		chinese := read(t, wikiFile(page+"-zh-CN.md"))
		for name, content := range map[string]string{"English": english, "Chinese": chinese} {
			for _, fragment := range []string{
				"[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/" + page + ")",
				"[简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/" + page + "-zh-CN)",
			} {
				if !strings.Contains(content, fragment) {
					t.Errorf("%s %s page lacks language link %q", page, name, fragment)
				}
			}
		}
	}
}

func TestWikiSidebarLinksEveryPageOnce(t *testing.T) {
	content := read(t, wikiFile("_Sidebar.md"))
	for _, page := range wikiPages {
		for _, slug := range []string{page, page + "-zh-CN"} {
			link := "](https://github.com/Sakuya1998/aws-cost-exporter/wiki/" + slug + ")"
			if count := strings.Count(content, link); count != 1 {
				t.Errorf("sidebar link %s occurs %d times", link, count)
			}
		}
	}
	for _, heading := range []string{"Getting started", "AWS data", "Observability", "Operations", "Development", "入门", "AWS 数据", "可观测性", "运维", "开发"} {
		if !strings.Contains(content, heading) {
			t.Errorf("sidebar lacks section %q", heading)
		}
	}
}

func TestWikiLinksReferenceManagedPages(t *testing.T) {
	managed := make(map[string]struct{}, len(wikiPages)*2)
	for _, page := range wikiPages {
		managed[page] = struct{}{}
		managed[page+"-zh-CN"] = struct{}{}
	}
	linkPattern := regexp.MustCompile(`https://github\.com/Sakuya1998/aws-cost-exporter/wiki/([A-Za-z0-9-]+)`)
	entries, err := os.ReadDir(filepath.Join("..", "..", "docs", "wiki"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" || entry.Name() == "README.md" {
			continue
		}
		content := read(t, wikiFile(entry.Name()))
		for _, match := range linkPattern.FindAllStringSubmatch(content, -1) {
			if _, ok := managed[match[1]]; !ok {
				t.Errorf("%s links unmanaged Wiki page %q", entry.Name(), match[1])
			}
		}
	}
}

func TestWikiDocumentsCurrentV030Contracts(t *testing.T) {
	configuration := read(t, wikiFile("Configuration-Reference.md"))
	for _, fragment := range []string{
		"v0.3.0", "server", "log", "aws", "targets", "collection", "cache", "telemetry",
		"--check-config", "AWS_COST_EXPORTER_SERVER__LISTEN_ADDRESS", "exact", "max_currencies",
	} {
		if !strings.Contains(configuration, fragment) {
			t.Errorf("configuration reference lacks %q", fragment)
		}
	}

	cur := read(t, wikiFile("CUR-and-Athena.md"))
	for _, fragment := range []string{
		"Data Exports", "S3", "Glue", "Athena", "StartQueryExecution", "GetQueryExecution",
		"GetQueryResults", "StopQueryExecution", "output_location", "does not create", "arbitrary SQL",
	} {
		if !strings.Contains(cur, fragment) {
			t.Errorf("CUR guide lacks %q", fragment)
		}
	}

	operations := read(t, wikiFile("Operations-and-Cost-Control.md"))
	for _, fragment := range []string{
		"/ready", "last successful snapshot", "replicaCount: 1", "USD 0.01", "Athena", "shutdown_timeout",
	} {
		if !strings.Contains(operations, fragment) {
			t.Errorf("operations guide lacks %q", fragment)
		}
	}
}

func TestWikiTracksProductionMetricNames(t *testing.T) {
	english := read(t, wikiFile("Metrics-Reference.md"))
	chinese := read(t, wikiFile("Metrics-Reference-zh-CN.md"))
	sources := []struct {
		file, prefix, pattern string
	}{
		{"cost.go", "aws_cost_", `costDesc\("([^"]+)"`},
		{"cost.go", "aws_cost_tag_", `tagCostDesc\("([^"]+)"`},
		{"cost.go", "aws_budget_", `budgetDesc\("([^"]+)"`},
		{"cost.go", "aws_commitment_", `commitmentDesc\("([^"]+)"`},
		{"exporter.go", "aws_cost_exporter_", `(?:selfDesc|counter|histogram)\("([^"]+)"`},
	}
	for _, source := range sources {
		content := read(t, filepath.Join("..", "..", "internal", "metrics", source.file))
		for _, match := range regexp.MustCompile(source.pattern).FindAllStringSubmatch(content, -1) {
			metric := source.prefix + match[1]
			for language, page := range map[string]string{"English": english, "Chinese": chinese} {
				if !strings.Contains(page, metric) {
					t.Errorf("%s metrics reference lacks %q", language, metric)
				}
			}
		}
	}
	for _, metric := range []string{
		"aws_cost_account_info",
		"aws_cost_anomaly_active",
		"aws_cost_anomaly_count",
		"aws_cost_anomaly_impact_amount",
		"aws_cost_anomaly_last_detected_timestamp_seconds",
		"aws_cost_exporter_scheduler_shutdown_timeouts_total",
	} {
		if !strings.Contains(english, metric) || !strings.Contains(chinese, metric) {
			t.Errorf("metrics references lack explicit metric %q", metric)
		}
	}
}

func TestCurrentDocumentationHasNoStaleReleaseStateOrSecrets(t *testing.T) {
	paths := []string{
		filepath.Join("..", "..", "README.md"),
		filepath.Join("..", "..", "ROADMAP.md"),
		filepath.Join("..", "..", "ARCHITECTURE.md"),
		filepath.Join("..", "..", "docs", "operations", "logging.md"),
		filepath.Join("..", "..", "docs", "operations", "troubleshooting.md"),
	}
	for _, page := range wikiPages {
		paths = append(paths, wikiFile(page+".md"), wikiFile(page+"-zh-CN.md"))
	}
	for _, path := range paths {
		content := read(t, path)
		for _, forbidden := range []string{"pending PR", "pending release", "codex/v0.3-development", "AKIA"} {
			if strings.Contains(content, forbidden) {
				t.Errorf("%s contains forbidden current-doc fragment %q", path, forbidden)
			}
		}
		if regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----`).MatchString(content) {
			t.Errorf("%s contains a private key", path)
		}
	}
}

func wikiFile(name string) string {
	return filepath.Join("..", "..", "docs", "wiki", name)
}
