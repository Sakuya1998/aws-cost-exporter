package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

func TestExporterCollectsTargetStatusAndBoundedEvents(t *testing.T) {
	id := identity.CollectorID{Target: "payer-prod", Name: "total"}
	success := time.Unix(1000, 0).UTC()
	reader := staticStatusReader{view: ports.SnapshotView{Collectors: map[identity.CollectorID]ports.CollectorStatus{id: {LastAttempt: time.Unix(1100, 0).UTC(), LastSuccess: success, Up: true, Series: 4}}}}
	subject, err := NewExporter(reader, fixedClock{now: success.Add(5 * time.Minute)}, version.Info{Version: "v0.2.0", Revision: "abc123", GoVersion: "go1.24.0"}, []identity.CollectorID{id})
	if err != nil {
		t.Fatal(err)
	}
	subject.ObserveRefresh(id, "success", 3*time.Second)
	subject.ObserveRequest("payer-prod", "GetCostAndUsage", "success", 2*time.Second)
	subject.ObserveRetry("payer-prod", "GetCostAndUsage", "throttle")
	subject.ObserveRetry("payer-prod", "private", "private")
	subject.ObserveSkipped(id, "single_flight")
	subject.ObserveOverflow("payer-prod", "service", 5)
	subject.ObservePaginationPage("payer-prod", "GetCostAndUsage")
	subject.ObserveCachePublishError(id, "publish")
	subject.ObserveSchedulerShutdownTimeout()
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(subject)
	const expected = `
# HELP aws_cost_exporter_build_info Build metadata for aws-cost-exporter.
# TYPE aws_cost_exporter_build_info gauge
aws_cost_exporter_build_info{go_version="go1.24.0",revision="abc123",version="v0.2.0"} 1
# HELP aws_cost_exporter_cache_age_seconds Seconds since the target collector last succeeded.
# TYPE aws_cost_exporter_cache_age_seconds gauge
aws_cost_exporter_cache_age_seconds{collector="total",target="payer-prod"} 300
# HELP aws_cost_exporter_collector_up Whether the target collector's latest attempt succeeded.
# TYPE aws_cost_exporter_collector_up gauge
aws_cost_exporter_collector_up{collector="total",target="payer-prod"} 1
# HELP aws_cost_exporter_snapshot_series Current business series owned by the target collector.
# TYPE aws_cost_exporter_snapshot_series gauge
aws_cost_exporter_snapshot_series{collector="total",target="payer-prod"} 4
`
	if err := testutil.GatherAndCompare(registry, strings.NewReader(expected), "aws_cost_exporter_build_info", "aws_cost_exporter_cache_age_seconds", "aws_cost_exporter_collector_up", "aws_cost_exporter_snapshot_series"); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name   string
		metric prometheus.Collector
		want   float64
	}{{"refresh", subject.refresh.WithLabelValues("payer-prod", "total", "success"), 1}, {"request", subject.requests.WithLabelValues("payer-prod", "GetCostAndUsage", "success"), 1}, {"retry", subject.retries.WithLabelValues("payer-prod", "GetCostAndUsage", "throttle"), 1}, {"bounded", subject.retries.WithLabelValues("payer-prod", "unknown", "other"), 1}, {"skipped", subject.skipped.WithLabelValues("payer-prod", "total", "single_flight"), 1}, {"overflow", subject.overflow.WithLabelValues("payer-prod", "service"), 5}, {"pagination", subject.pagination.WithLabelValues("payer-prod", "GetCostAndUsage"), 1}, {"cache", subject.cachePublishErrors.WithLabelValues("payer-prod", "total", "publish"), 1}, {"shutdown", subject.shutdownTimeouts, 1}}
	for _, check := range checks {
		if got := testutil.ToFloat64(check.metric); got != check.want {
			t.Fatalf("%s=%v", check.name, got)
		}
	}
}

func TestNewExporterRejectsInvalidDependenciesAndIDs(t *testing.T) {
	if subject, err := NewExporter(nil, nil, version.Info{}, nil); subject != nil || err == nil {
		t.Fatal("accepted nil dependencies")
	}
	id := identity.CollectorID{Target: "a", Name: "total"}
	reader := staticStatusReader{}
	clock := fixedClock{}
	if subject, err := NewExporter(reader, clock, version.Info{}, []identity.CollectorID{id, id}); subject != nil || err == nil {
		t.Fatal("accepted duplicate ID")
	}
}

type staticStatusReader struct{ view ports.SnapshotView }

func (value staticStatusReader) Load() ports.SnapshotView { return value.view }

type fixedClock struct{ now time.Time }

func (value fixedClock) Now() time.Time { return value.now }
