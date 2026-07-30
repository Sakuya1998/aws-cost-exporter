package metrics

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

var updateMetricsContract = flag.Bool("update-contract", false, "rewrite the reviewed v1 metrics contract fixture")

type metricContract struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	VariableLabels []string `json:"variable_labels"`
	Unit           string   `json:"unit"`
}

type boundedMetricEnums struct {
	Operations      []string `json:"operations"`
	Providers       []string `json:"providers"`
	CostBases       []string `json:"cost_bases"`
	RefreshStatuses []string `json:"refresh_statuses"`
	RequestStatuses []string `json:"request_statuses"`
	RetryReasons    []string `json:"retry_reasons"`
}

type prometheusContract struct {
	Version      string             `json:"version"`
	Metrics      []metricContract   `json:"metrics"`
	BoundedEnums boundedMetricEnums `json:"bounded_enums"`
}

func TestV1PrometheusContract(t *testing.T) {
	registry, collectors := fullMetricsFixture(t)
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather full metrics fixture: %v", err)
	}
	descriptors := describeMetrics(t, collectors...)
	assertUniqueMetricLabelSets(t, families)
	assertNoForbiddenDescriptorLabels(t, descriptors)

	contract := buildV1PrometheusContract(t, families, descriptors)
	assertCostLabelPrefix(t, contract.Metrics)
	got, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		t.Fatalf("marshal v1 metrics contract: %v", err)
	}

	fixturePath := filepath.Join("testdata", "v1", "metrics-contract.json")
	if *updateMetricsContract {
		if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
			t.Fatalf("create metrics contract fixture directory: %v", err)
		}
		if err := os.WriteFile(fixturePath, got, 0o600); err != nil {
			t.Fatalf("write metrics contract fixture: %v", err)
		}
	}

	want, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read reviewed v1 metrics contract fixture: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("v1 prometheus contract changed; review the change, then run go test ./internal/metrics -run TestV1PrometheusContract -args -update-contract\n%s", formatMetricContractDiff(want, got))
	}
}

func TestV1BoundedMetricEnums(t *testing.T) {
	enums := v1BoundedMetricEnums()
	for _, operation := range enums.Operations[:len(enums.Operations)-1] {
		if got := boundedOperation(operation); got != operation {
			t.Errorf("boundedOperation(%q) = %q", operation, got)
		}
	}
	for _, provider := range enums.Providers {
		if !cost.Provider(provider).Valid() {
			t.Errorf("provider %q is not valid", provider)
		}
	}
	for _, basis := range enums.CostBases {
		if !cost.Basis(basis).Valid() {
			t.Errorf("cost basis %q is not valid", basis)
		}
	}
	for _, status := range enums.RefreshStatuses[:len(enums.RefreshStatuses)-1] {
		if got := bounded(status, "success", "error", "canceled"); got != status {
			t.Errorf("refresh status %q mapped to %q", status, got)
		}
	}
	for _, status := range enums.RequestStatuses[:len(enums.RequestStatuses)-1] {
		if got := bounded(status, "success", "error", "canceled", "throttle"); got != status {
			t.Errorf("request status %q mapped to %q", status, got)
		}
	}
	for _, reason := range enums.RetryReasons {
		if got := boundedDefault(reason, "other", "throttle", "timeout", "other"); got != reason {
			t.Errorf("retry reason %q mapped to %q", reason, got)
		}
	}

	private := "private-random-8f5d1d7b"
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"operation", boundedOperation(private), "unknown"},
		{"refresh status", bounded(private, "success", "error", "canceled"), "unknown"},
		{"request status", bounded(private, "success", "error", "canceled", "throttle"), "unknown"},
		{"retry reason", boundedDefault(private, "other", "throttle", "timeout", "other"), "other"},
	}
	for _, test := range tests {
		if test.got != test.want || test.got == private {
			t.Errorf("private %s mapped to %q, want %q", test.name, test.got, test.want)
		}
	}
	if cost.Provider(private).Valid() || cost.Basis(private).Valid() {
		t.Fatal("private provider or cost basis accepted")
	}
}

func fullMetricsFixture(t *testing.T) (*prometheus.Registry, []prometheus.Collector) {
	t.Helper()
	costCollector, err := NewCostCollector(staticStore{snapshot: businessSnapshot(t)})
	if err != nil {
		t.Fatal(err)
	}

	target := identity.TargetID("payer-prod")
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	names := []string{"total", "service", "region", "account", "forecast", "tags", "organizations", "budgets", "commitments", "anomalies", "cur"}
	ids := make([]identity.CollectorID, 0, len(names))
	statuses := make(map[identity.CollectorID]ports.CollectorStatus, len(names))
	for index, name := range names {
		id := identity.CollectorID{Target: target, Name: name}
		ids = append(ids, id)
		statuses[id] = ports.CollectorStatus{LastAttempt: now.Add(-time.Minute), LastSuccess: now.Add(-2 * time.Minute), Up: true, Series: index + 1}
	}
	exporter, err := NewExporter(staticStatusReader{view: ports.SnapshotView{Collectors: statuses}}, fixedClock{now: now}, version.Info{Version: "v1.0.0", Revision: "contract", GoVersion: "go1.24.0"}, ids)
	if err != nil {
		t.Fatal(err)
	}
	exporter.ObserveRefresh(ids[0], "success", time.Second)
	exporter.ObserveRequest(target, "GetCostAndUsage", "success", time.Second)
	exporter.ObserveRetry(target, "GetCostAndUsage", "throttle")
	exporter.ObserveSkipped(ids[0], "single_flight")
	exporter.ObserveOverflow(target, "service", 1)
	exporter.ObservePaginationPage(target, "GetCostAndUsage")
	exporter.ObserveCachePublishError(ids[0], "publish")
	exporter.ObserveSchedulerShutdownTimeout()

	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(costCollector, exporter)
	return registry, []prometheus.Collector{costCollector, exporter}
}

func buildV1PrometheusContract(t *testing.T, families []*dto.MetricFamily, descriptors map[string][]string) prometheusContract {
	t.Helper()
	contract := prometheusContract{Version: "v1.0.0", BoundedEnums: v1BoundedMetricEnums()}
	seenFamilies := make(map[string]struct{}, len(families))
	for _, family := range families {
		name := family.GetName()
		labels, exists := descriptors[name]
		if !exists {
			t.Fatalf("gathered metric family %q has no descriptor", name)
		}
		seenFamilies[name] = struct{}{}
		contract.Metrics = append(contract.Metrics, metricContract{
			Name:           name,
			Type:           strings.ToLower(family.GetType().String()),
			VariableLabels: append([]string{}, labels...),
			Unit:           v1MetricUnit(t, name, family.GetUnit()),
		})
	}
	for name := range descriptors {
		if _, exists := seenFamilies[name]; !exists {
			t.Fatalf("descriptor %q did not produce a metric family in the full fixture", name)
		}
	}
	sort.Slice(contract.Metrics, func(left, right int) bool {
		return contract.Metrics[left].Name < contract.Metrics[right].Name
	})
	return contract
}

func v1MetricUnit(t *testing.T, name, declared string) string {
	t.Helper()
	if declared != "" {
		return declared
	}
	switch {
	case strings.HasSuffix(name, "_amount"):
		return "currency"
	case strings.HasSuffix(name, "_timestamp_seconds"), strings.HasSuffix(name, "_duration_seconds"), strings.HasSuffix(name, "_age_seconds"):
		return "seconds"
	case strings.HasSuffix(name, "_ratio"):
		return "ratio"
	case strings.HasSuffix(name, "_hours"):
		return "hours"
	case strings.HasSuffix(name, "_up"), strings.HasSuffix(name, "_active"):
		return "boolean"
	case strings.HasSuffix(name, "_info"):
		return "info"
	case strings.HasSuffix(name, "_total"), strings.HasSuffix(name, "_count"), strings.HasSuffix(name, "_series"):
		return "count"
	default:
		t.Fatalf("metric %q has no declared or stable inferred unit", name)
		return ""
	}
}

func v1BoundedMetricEnums() boundedMetricEnums {
	return boundedMetricEnums{
		Operations: []string{
			"AssumeRole", "GetCallerIdentity", "GetCostAndUsage", "GetCostForecast", "ListAccounts", "DescribeOrganization", "DescribeBudgets",
			"GetSavingsPlansUtilization", "GetSavingsPlansCoverage", "GetReservationUtilization", "GetReservationCoverage", "GetAnomalies",
			"StartQueryExecution", "GetQueryExecution", "GetQueryResults", "StopQueryExecution", "unknown",
		},
		Providers:       []string{"cost_explorer", "cur_athena"},
		CostBases:       []string{"unblended", "amortized", "net"},
		RefreshStatuses: []string{"success", "error", "canceled", "unknown"},
		RequestStatuses: []string{"success", "error", "canceled", "throttle", "unknown"},
		RetryReasons:    []string{"throttle", "timeout", "other"},
	}
}

func describeMetrics(t *testing.T, collectors ...prometheus.Collector) map[string][]string {
	t.Helper()
	output := make(chan *prometheus.Desc, 128)
	for _, collector := range collectors {
		collector.Describe(output)
	}
	close(output)

	result := make(map[string][]string)
	for descriptor := range output {
		name, labels := parseDescriptor(t, descriptor.String())
		if previous, exists := result[name]; exists && !reflect.DeepEqual(previous, labels) {
			t.Fatalf("metric %q has inconsistent descriptor labels %v and %v", name, previous, labels)
		}
		result[name] = labels
	}
	return result
}

func parseDescriptor(t *testing.T, value string) (string, []string) {
	t.Helper()
	const namePrefix = `Desc{fqName: `
	const nameSuffix = `, help: `
	const labelMarker = `, constLabels: {}, variableLabels: {`
	if !strings.HasPrefix(value, namePrefix) || !strings.HasSuffix(value, "}}") {
		t.Fatalf("unrecognized prometheus descriptor %q", value)
	}
	nameEnd := strings.Index(value, nameSuffix)
	labelStart := strings.LastIndex(value, labelMarker)
	if nameEnd < len(namePrefix) || labelStart < nameEnd {
		t.Fatalf("descriptor cannot be normalized without losing contract data: %q", value)
	}
	name, err := strconv.Unquote(value[len(namePrefix):nameEnd])
	if err != nil {
		t.Fatalf("parse descriptor name from %q: %v", value, err)
	}
	labelText := value[labelStart+len(labelMarker) : len(value)-2]
	if labelText == "" {
		return name, []string{}
	}
	return name, strings.Split(labelText, ",")
}

func assertCostLabelPrefix(t *testing.T, metrics []metricContract) {
	t.Helper()
	want := []string{"target", "provider", "cost_basis", "currency"}
	matched := 0
	for _, metric := range metrics {
		if !contains(metric.VariableLabels, "cost_basis") {
			continue
		}
		matched++
		if len(metric.VariableLabels) < len(want) || !reflect.DeepEqual(metric.VariableLabels[:len(want)], want) {
			t.Errorf("metric %s labels = %v, want prefix %v", metric.Name, metric.VariableLabels, want)
		}
	}
	if matched != 13 {
		t.Errorf("cost label prefix checked on %d metrics, want 13", matched)
	}
}

func assertUniqueMetricLabelSets(t *testing.T, families []*dto.MetricFamily) {
	t.Helper()
	seen := make(map[string]struct{})
	for _, family := range families {
		for _, metric := range family.Metric {
			labels := append([]*dto.LabelPair(nil), metric.Label...)
			sort.Slice(labels, func(left, right int) bool {
				if labels[left].GetName() != labels[right].GetName() {
					return labels[left].GetName() < labels[right].GetName()
				}
				return labels[left].GetValue() < labels[right].GetValue()
			})
			encoded, err := json.Marshal(labels)
			if err != nil {
				t.Fatalf("encode labels for %s: %v", family.GetName(), err)
			}
			key := family.GetName() + "\x00" + string(encoded)
			if _, duplicate := seen[key]; duplicate {
				t.Fatalf("metric family %s contains duplicate label set %s", family.GetName(), encoded)
			}
			seen[key] = struct{}{}
		}
	}
}

func assertNoForbiddenDescriptorLabels(t *testing.T, descriptors map[string][]string) {
	t.Helper()
	forbidden := map[string]struct{}{
		"email": {}, "anomaly_id": {}, "reservation_id": {}, "plan_id": {}, "role_arn": {}, "external_id": {}, "error": {}, "raw_error": {}, "request_id": {},
	}
	for name, labels := range descriptors {
		for _, label := range labels {
			if _, exists := forbidden[label]; exists {
				t.Errorf("metric %s exposes forbidden label %q", name, label)
			}
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func formatMetricContractDiff(want, got []byte) string {
	return fmt.Sprintf("want:\n%s\n\ngot:\n%s", want, got)
}
