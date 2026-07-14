package dashboard_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type dashboard struct {
	Title         string  `json:"title"`
	UID           string  `json:"uid"`
	SchemaVersion int     `json:"schemaVersion"`
	Timezone      string  `json:"timezone"`
	Panels        []panel `json:"panels"`
	Templating    struct {
		List []variable `json:"list"`
	} `json:"templating"`
}

type panel struct {
	Title   string   `json:"title"`
	Type    string   `json:"type"`
	Targets []target `json:"targets"`
	Options struct {
		Content string `json:"content"`
	} `json:"options"`
}

type target struct {
	Expr string `json:"expr"`
}

type variable struct {
	Name       string `json:"name"`
	IncludeAll bool   `json:"includeAll"`
	Query      struct {
		Query string `json:"query"`
	} `json:"query"`
}

func TestDashboardContainsRequiredPanelsAndVariables(t *testing.T) {
	value := loadDashboard(t)
	if value.Title != "AWS Cost Exporter" || value.UID != "aws-cost-exporter" ||
		value.SchemaVersion < 38 || value.Timezone != "utc" {
		t.Fatalf("dashboard metadata = title %q uid %q schema %d timezone %q", value.Title, value.UID, value.SchemaVersion, value.Timezone)
	}
	variables := make(map[string]variable, len(value.Templating.List))
	for _, item := range value.Templating.List {
		variables[item.Name] = item
	}
	for _, name := range []string{"job", "instance", "currency", "aws_service", "aws_region", "linked_account_id"} {
		item, exists := variables[name]
		if !exists || !item.IncludeAll || !strings.Contains(item.Query.Query, "label_values(") {
			t.Errorf("variable %s is missing label_values/includeAll", name)
		}
	}
	required := map[string]string{
		"Today accumulated": "stat", "Month to date": "stat",
		"Estimated month end": "stat", "Remaining-month forecast": "stat",
		"Data age": "stat", "Daily cost scrape history": "timeseries",
		"MTD and estimated month-end bounds": "timeseries", "Top 10 services": "bargauge",
		"Top 10 regions": "bargauge", "Top 10 accounts": "bargauge",
		"Collector status": "table", "Dimension overflow": "table",
		"AWS API requests": "timeseries", "AWS API error ratio": "timeseries",
		"AWS API p99 latency": "timeseries", "AWS API retries": "timeseries",
		"Skipped refreshes": "timeseries", "Cost data semantics": "text",
	}
	for _, item := range value.Panels {
		if want, exists := required[item.Title]; exists {
			if item.Type != want {
				t.Errorf("panel %q type=%q want=%q", item.Title, item.Type, want)
			}
			delete(required, item.Title)
		}
	}
	if len(required) != 0 {
		t.Errorf("required panels missing: %v", required)
	}
}

func TestDashboardPromQLUsesOnlyRealCurrencySafeMetrics(t *testing.T) {
	value := loadDashboard(t)
	contracts := metricContracts(t)
	metricPattern := regexp.MustCompile(`aws_cost(?:_exporter)?_[a-zA-Z0-9_:]+`)
	selectorPattern := regexp.MustCompile(`(aws_cost(?:_exporter)?_[a-zA-Z0-9_:]+)\{([^}]*)\}`)
	labelPattern := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\s*(?:=|!=|=~|!~)`)
	for _, item := range value.Panels {
		for _, query := range item.Targets {
			if query.Expr == "" || !balancedPromQL(query.Expr) {
				t.Errorf("panel %q has structurally invalid PromQL: %s", item.Title, query.Expr)
			}
			for _, name := range metricPattern.FindAllString(query.Expr, -1) {
				if _, exists := contracts[name]; !exists {
					t.Errorf("panel %q references unknown metric %q", item.Title, name)
				}
			}
			for _, selector := range selectorPattern.FindAllStringSubmatch(query.Expr, -1) {
				for _, match := range labelPattern.FindAllStringSubmatch(selector[2], -1) {
					if match[1] == "job" || match[1] == "instance" {
						continue
					}
					if _, exists := contracts[selector[1]][match[1]]; !exists {
						t.Errorf("panel %q uses unknown label %q on %s", item.Title, match[1], selector[1])
					}
				}
			}
			if strings.Contains(query.Expr, "aws_cost_") &&
				!strings.Contains(query.Expr, "aws_cost_exporter_") {
				for _, filter := range []string{`job=~"$job"`, `instance=~"$instance"`, `currency=~"$currency"`} {
					if !strings.Contains(query.Expr, filter) {
						t.Errorf("panel %q business query lacks %s: %s", item.Title, filter, query.Expr)
					}
				}
			}
			if strings.HasPrefix(item.Title, "Top 10 ") &&
				!strings.Contains(query.Expr, "topk by (currency)") {
				t.Errorf("panel %q ranks different currencies together: %s", item.Title, query.Expr)
			}
		}
	}
	serialized, _ := json.Marshal(value)
	if !strings.Contains(string(serialized), "increase(aws_cost_exporter_dimension_overflow_values_total") {
		t.Error("overflow counter is not queried with increase")
	}
}

func TestMonthEndEstimateDoesNotCountTodayTwice(t *testing.T) {
	value := loadDashboard(t)
	for _, item := range value.Panels {
		if item.Title != "Estimated month end" &&
			item.Title != "MTD and estimated month-end bounds" {
			continue
		}
		for _, query := range item.Targets {
			if !strings.Contains(query.Expr, "aws_cost_month_forecast_") {
				continue
			}
			for _, term := range []string{
				"aws_cost_month_to_date_amount",
				"aws_cost_daily_amount",
				" - sum by (currency)",
				" + sum by (currency)",
			} {
				if !strings.Contains(query.Expr, term) {
					t.Errorf("panel %q forecast query lacks overlap-safe term %q: %s", item.Title, term, query.Expr)
				}
			}
		}
	}
}

func loadDashboard(t *testing.T) dashboard {
	t.Helper()
	path := filepath.Join("..", "..", "dashboards", "grafana", "aws-cost-exporter.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	var value dashboard
	if err := json.Unmarshal(content, &value); err != nil {
		t.Fatalf("parse dashboard JSON: %v", err)
	}
	return value
}

func metricContracts(t *testing.T) map[string]map[string]struct{} {
	t.Helper()
	result := make(map[string]map[string]struct{})
	costSource, err := os.ReadFile(filepath.Join("..", "..", "internal", "metrics", "cost.go"))
	if err != nil {
		t.Fatalf("read cost metric contract: %v", err)
	}
	costPattern := regexp.MustCompile(`newDesc\("([^"]+)",\s*"[^"]*",\s*"([^"]*)"\)`)
	for _, match := range costPattern.FindAllStringSubmatch(string(costSource), -1) {
		result["aws_cost_"+match[1]] = labels("currency", match[2])
	}
	exporterSource, err := os.ReadFile(filepath.Join("..", "..", "internal", "metrics", "exporter.go"))
	if err != nil {
		t.Fatalf("read exporter metric contract: %v", err)
	}
	exporterPattern := regexp.MustCompile(`(selfDesc|counter|histogram)\("([^"]+)",\s*"[^"]*",\s*\[\]string\{([^}]*)\}`)
	for _, match := range exporterPattern.FindAllStringSubmatch(string(exporterSource), -1) {
		name := "aws_cost_exporter_" + match[2]
		result[name] = labels(strings.ReplaceAll(match[3], `"`, ""))
		if match[1] == "histogram" {
			result[name+"_bucket"] = labels(strings.ReplaceAll(match[3], `"`, ""), "le")
		}
	}
	return result
}

func labels(groups ...string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, group := range groups {
		for _, label := range strings.Split(group, ",") {
			if label = strings.TrimSpace(label); label != "" {
				result[label] = struct{}{}
			}
		}
	}
	return result
}

func balancedPromQL(expression string) bool {
	stack := make([]rune, 0)
	pairs := map[rune]rune{')': '(', ']': '[', '}': '{'}
	for _, character := range expression {
		switch character {
		case '(', '[', '{':
			stack = append(stack, character)
		case ')', ']', '}':
			if len(stack) == 0 || stack[len(stack)-1] != pairs[character] {
				return false
			}
			stack = stack[:len(stack)-1]
		}
	}
	return len(stack) == 0 && strings.Count(expression, `"`)%2 == 0
}
