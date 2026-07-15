package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

// TestExporterCollectsStatusEventsAndBoundedLabels verifies self-observability.
func TestExporterCollectsStatusEventsAndBoundedLabels(t *testing.T) {
	success := time.Unix(1000, 0).UTC()
	reader := staticStatusReader{view: ports.SnapshotView{Collectors: map[string]ports.CollectorStatus{
		"total": {LastAttempt: time.Unix(1100, 0).UTC(), LastSuccess: success, Up: true, Series: 4},
	}}}
	subject, err := NewExporter(
		reader, fixedClock{now: success.Add(5 * time.Minute)},
		version.Info{Version: "v0.1.0", Revision: "abc123", GoVersion: "go1.24.0"},
		[]string{"total"},
	)
	if err != nil {
		t.Fatalf("NewExporter() error = %v", err)
	}
	subject.ObserveRefresh("total", "success", 3*time.Second)
	subject.ObserveRefresh("unregistered", "error", time.Second)
	subject.ObserveRequest("GetCostAndUsage", "success", 2*time.Second)
	subject.ObserveRetry("GetCostAndUsage", "throttle")
	subject.ObserveRetry("private-operation", "private-error")
	subject.ObserveSkipped("total", "single_flight")
	subject.ObserveOverflow("service", 5)
	subject.ObservePaginationPage("GetCostAndUsage")
	subject.ObserveCachePublishError("total", "publish")
	subject.ObserveSchedulerShutdownTimeout()

	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(subject)
	const expected = `
# HELP aws_cost_exporter_build_info Build metadata for aws-cost-exporter.
# TYPE aws_cost_exporter_build_info gauge
aws_cost_exporter_build_info{go_version="go1.24.0",revision="abc123",version="v0.1.0"} 1
# HELP aws_cost_exporter_cache_age_seconds Seconds since the collector last succeeded.
# TYPE aws_cost_exporter_cache_age_seconds gauge
aws_cost_exporter_cache_age_seconds{collector="total"} 300
# HELP aws_cost_exporter_collector_up Whether the collector's latest attempt succeeded.
# TYPE aws_cost_exporter_collector_up gauge
aws_cost_exporter_collector_up{collector="total"} 1
# HELP aws_cost_exporter_snapshot_series Current business series owned by the collector.
# TYPE aws_cost_exporter_snapshot_series gauge
aws_cost_exporter_snapshot_series{collector="total"} 4
`
	names := []string{
		"aws_cost_exporter_build_info", "aws_cost_exporter_cache_age_seconds",
		"aws_cost_exporter_collector_up",
		"aws_cost_exporter_snapshot_series",
	}
	if err := testutil.GatherAndCompare(registry, strings.NewReader(expected), names...); err != nil {
		t.Fatalf("GatherAndCompare() error = %v", err)
	}
	families, err := registry.Gather()
	if err != nil || len(families) != 16 {
		t.Fatalf("Gather() returned %d families, %v; want 16", len(families), err)
	}
	checks := []struct {
		name   string
		metric prometheus.Collector
		want   float64
	}{
		{"refresh", subject.refresh.WithLabelValues("total", "success"), 1},
		{"request", subject.requests.WithLabelValues("GetCostAndUsage", "success"), 1},
		{"retry", subject.retries.WithLabelValues("GetCostAndUsage", "throttle"), 1},
		{"bounded retry", subject.retries.WithLabelValues("unknown", "other"), 1},
		{"skipped", subject.skipped.WithLabelValues("total", "single_flight"), 1},
		{"overflow", subject.overflow.WithLabelValues("service"), 5},
		{"pagination", subject.pagination.WithLabelValues("GetCostAndUsage"), 1},
		{"cache publish", subject.cachePublishErrors.WithLabelValues("total", "publish"), 1},
		{"shutdown timeout", subject.shutdownTimeouts, 1},
	}
	for _, check := range checks {
		if got := testutil.ToFloat64(check.metric); got != check.want {
			t.Fatalf("%s = %v, want %v", check.name, got, check.want)
		}
	}
}

// TestNewExporterRejectsInvalidDependencies verifies fail-fast construction.
func TestNewExporterRejectsInvalidDependencies(t *testing.T) {
	if subject, err := NewExporter(nil, nil, version.Info{}, nil); subject != nil || err == nil {
		t.Fatalf("NewExporter(invalid) = %#v, %v; want error", subject, err)
	}
}

type staticStatusReader struct{ view ports.SnapshotView }

func (reader staticStatusReader) Load() ports.SnapshotView { return reader.view }

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }
