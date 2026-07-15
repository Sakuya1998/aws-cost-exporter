package rules_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type ruleFile struct {
	Groups []ruleGroup `yaml:"groups"`
}

type ruleGroup struct {
	Name  string `yaml:"name"`
	Rules []rule `yaml:"rules"`
}

type rule struct {
	Alert       string            `yaml:"alert"`
	Record      string            `yaml:"record"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

func TestRulesContainSafeRecordingsAndAlerts(t *testing.T) {
	value, content := loadRules(t)
	records, alerts := make(map[string]rule), make(map[string]rule)
	for _, group := range value.Groups {
		for _, item := range group.Rules {
			if item.Record != "" {
				records[item.Record] = item
			}
			if item.Alert != "" {
				alerts[item.Alert] = item
			}
		}
	}
	requireExpr(t, records, "aws_cost:month_to_date:sum_by_currency",
		"sum by (job, instance, currency) (aws_cost_month_to_date_amount)")
	requireExpr(t, records, "aws_cost_exporter:max_cache_age_seconds",
		"max by (job, instance) (aws_cost_exporter_cache_age_seconds)")
	requireAlert(t, alerts, "AWSCostExporterDataStale", "30m", "aws_cost_exporter:max_cache_age_seconds > 43200")
	requireAlert(t, alerts, "AWSCostExporterCollectorDown", "30m",
		"aws_cost_exporter_collector_up == 0")
	requireAlert(t, alerts, "AWSCostExplorerPaginationSpike", "15m",
		`sum by (job, instance) (rate(aws_cost_exporter_pagination_pages_total[1h])) > 100`)
	requireAlert(t, alerts, "AWSCostExplorerThrottleSustained", "15m",
		`sum by (job, instance) (rate(aws_cost_exporter_aws_api_requests_total{status="throttle"}[15m])) > 0`)
	requireAlert(t, alerts, "AWSDailyCostSpike", "2h",
		"aws_cost_daily_amount > 1.5 * avg_over_time(aws_cost_daily_amount[7d])")
	if _, enabled := alerts["AWSMonthlyCostForecastAboveBudget"]; enabled {
		t.Error("budget example must not be enabled by default")
	}
	text := string(content)
	for _, fragment := range []string{
		"# - alert: AWSMonthlyCostForecastAboveBudget",
		"aws_cost_month_to_date_amount{currency=\"USD\"}",
		"- aws_cost_daily_amount{currency=\"USD\"}",
		"+ aws_cost_month_forecast_mean_amount{currency=\"USD\"}",
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("disabled budget example lacks %q", fragment)
		}
	}
}

func TestRuleExpressionsReferenceRealMetrics(t *testing.T) {
	value, _ := loadRules(t)
	known := productionMetrics(t)
	for _, group := range value.Groups {
		for _, item := range group.Rules {
			if item.Record != "" {
				known[item.Record] = struct{}{}
			}
		}
	}
	metricPattern := regexp.MustCompile(`aws_cost(?:_exporter)?(?::|_)[a-zA-Z0-9_:]+`)
	for _, group := range value.Groups {
		for _, item := range group.Rules {
			for _, name := range metricPattern.FindAllString(item.Expr, -1) {
				if _, exists := known[name]; !exists {
					t.Errorf("rule %q references unknown metric %q", ruleName(item), name)
				}
			}
			if strings.Contains(item.Expr, "sum") && strings.Contains(item.Expr, "aws_cost_") &&
				!strings.Contains(item.Expr, "aws_cost_exporter_") &&
				!strings.Contains(item.Expr, "currency") {
				t.Errorf("rule %q aggregates costs across currencies", ruleName(item))
			}
		}
	}
}

func TestPromtoolCheckRules(t *testing.T) {
	tool, err := exec.LookPath("promtool")
	if err != nil {
		t.Skip("promtool is not installed")
	}
	output, err := exec.Command(tool, "check", "rules", rulesPath()).CombinedOutput()
	if err != nil {
		t.Fatalf("promtool check rules: %v\n%s", err, output)
	}
}

func loadRules(t *testing.T) (ruleFile, []byte) {
	t.Helper()
	content, err := os.ReadFile(rulesPath())
	if err != nil {
		t.Fatalf("read rules: %v", err)
	}
	var value ruleFile
	if err := yaml.Unmarshal(content, &value); err != nil {
		t.Fatalf("parse rules: %v", err)
	}
	return value, content
}

func rulesPath() string {
	return filepath.Join("..", "..", "rules", "prometheus", "aws-cost-exporter.rules.yaml")
}

func requireExpr(t *testing.T, values map[string]rule, name, expression string) {
	t.Helper()
	if item, exists := values[name]; !exists || item.Expr != expression {
		t.Errorf("recording rule %q expr=%q want=%q", name, item.Expr, expression)
	}
}

func requireAlert(t *testing.T, values map[string]rule, name, duration, expression string) {
	t.Helper()
	item, exists := values[name]
	if !exists || item.For != duration || item.Expr != expression ||
		item.Labels["severity"] != "warning" || item.Annotations["summary"] == "" ||
		item.Annotations["description"] == "" {
		t.Errorf("alert %q does not satisfy its contract: %+v", name, item)
	}
}

func productionMetrics(t *testing.T) map[string]struct{} {
	t.Helper()
	result := make(map[string]struct{})
	sources := []struct {
		path, prefix, pattern string
	}{
		{"cost.go", "aws_cost_", `newDesc\("([^"]+)"`},
		{"exporter.go", "aws_cost_exporter_", `(?:selfDesc|counter|histogram)\("([^"]+)"`},
	}
	for _, source := range sources {
		content, err := os.ReadFile(filepath.Join("..", "..", "internal", "metrics", source.path))
		if err != nil {
			t.Fatalf("read metric contract: %v", err)
		}
		for _, match := range regexp.MustCompile(source.pattern).FindAllStringSubmatch(string(content), -1) {
			result[source.prefix+match[1]] = struct{}{}
		}
	}
	return result
}

func ruleName(item rule) string {
	if item.Alert != "" {
		return item.Alert
	}
	return item.Record
}
