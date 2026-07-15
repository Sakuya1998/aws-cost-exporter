package metrics

import (
	"errors"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

// StatusReader supplies one atomic cache and collector status view.
type StatusReader interface {
	Load() ports.SnapshotView
}

// ErrInvalidExporter indicates unsafe telemetry dependencies or identities.
var ErrInvalidExporter = errors.New("invalid exporter metrics configuration")

// Exporter exposes cache state and bounded operational observations.
type Exporter struct {
	reader StatusReader
	clock  ports.Clock
	names  []string
	known  map[string]struct{}
	build  version.Info

	up, success, attempt, age, series, buildInfo  *prometheus.Desc
	refresh, requests, retries, skipped, overflow *prometheus.CounterVec
	pagination, cachePublishErrors                *prometheus.CounterVec
	shutdownTimeouts                              prometheus.Counter
	refreshDuration, requestDuration              *prometheus.HistogramVec
	events                                        []prometheus.Collector
}

// NewExporter constructs the complete self-observability metric set.
func NewExporter(reader StatusReader, clock ports.Clock, build version.Info, names []string) (*Exporter, error) {
	if reader == nil || clock == nil || len(names) == 0 {
		return nil, ErrInvalidExporter
	}
	known := make(map[string]struct{}, len(names))
	normalized := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if _, exists := known[name]; name == "" || exists {
			return nil, ErrInvalidExporter
		}
		known[name] = struct{}{}
		normalized = append(normalized, name)
	}
	names = normalized
	exporter := &Exporter{reader: reader, clock: clock, names: names, known: known, build: build}
	exporter.up = selfDesc("collector_up", "Whether the collector's latest attempt succeeded.", []string{"collector"})
	exporter.success = selfDesc("last_success_timestamp_seconds", "Collector's latest success Unix timestamp.", []string{"collector"})
	exporter.attempt = selfDesc("last_attempt_timestamp_seconds", "Collector's latest attempt Unix timestamp.", []string{"collector"})
	exporter.age = selfDesc("cache_age_seconds", "Seconds since the collector last succeeded.", []string{"collector"})
	exporter.series = selfDesc("snapshot_series", "Current business series owned by the collector.", []string{"collector"})
	exporter.buildInfo = selfDesc("build_info", "Build metadata for aws-cost-exporter.", []string{"version", "revision", "go_version"})
	exporter.refresh = counter("refresh_total", "Collector refresh attempts.", []string{"collector", "status"})
	exporter.requests = counter("aws_api_requests_total", "Cost Explorer logical SDK operations.", []string{"operation", "status"})
	exporter.retries = counter("aws_api_retries_total", "Cost Explorer SDK retries.", []string{"operation", "reason"})
	exporter.skipped = counter("scheduler_skipped_runs_total", "Scheduler runs skipped before execution.", []string{"collector", "reason"})
	exporter.overflow = counter("dimension_overflow_values_total", "Dimension values processed into overflow during collection attempts.", []string{"dimension"})
	exporter.pagination = counter("pagination_pages_total", "Cost Explorer pagination pages read successfully.", []string{"operation"})
	exporter.cachePublishErrors = counter("cache_publish_errors_total", "Cache publish or failure-record errors.", []string{"collector", "operation"})
	exporter.shutdownTimeouts = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "aws_cost_exporter_scheduler_shutdown_timeouts_total",
		Help: "Scheduler shutdown waits that exceeded server.shutdown_timeout.",
	})
	exporter.refreshDuration = histogram("refresh_duration_seconds", "Collector refresh duration.", []string{"collector"}, []float64{1, 5, 10, 30, 60, 120, 300})
	exporter.requestDuration = histogram("aws_api_request_duration_seconds", "Cost Explorer logical SDK operation duration, including rate-limit waits and retries.", []string{"operation"}, []float64{.1, .5, 1, 2, 5, 10, 30, 60})
	exporter.events = []prometheus.Collector{
		exporter.refresh, exporter.requests, exporter.retries, exporter.skipped,
		exporter.overflow, exporter.pagination, exporter.cachePublishErrors,
		exporter.shutdownTimeouts,
		exporter.refreshDuration, exporter.requestDuration,
	}
	return exporter, nil
}

// Describe sends dynamic descriptors and delegated event descriptors.
func (exporter *Exporter) Describe(output chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{exporter.up, exporter.success, exporter.attempt, exporter.age, exporter.series, exporter.buildInfo} {
		output <- desc
	}
	for _, event := range exporter.events {
		event.Describe(output)
	}
}

// Collect emits one atomic cache view and accumulated event metrics.
func (exporter *Exporter) Collect(output chan<- prometheus.Metric) {
	view, now := exporter.reader.Load(), exporter.clock.Now()
	for _, name := range exporter.names {
		status := view.Collectors[name]
		output <- prometheus.MustNewConstMetric(exporter.up, prometheus.GaugeValue, boolFloat(status.Up), name)
		output <- prometheus.MustNewConstMetric(exporter.series, prometheus.GaugeValue, float64(status.Series), name)
		if !status.LastAttempt.IsZero() {
			output <- prometheus.MustNewConstMetric(exporter.attempt, prometheus.GaugeValue, float64(status.LastAttempt.Unix()), name)
		}
		if !status.LastSuccess.IsZero() {
			output <- prometheus.MustNewConstMetric(exporter.success, prometheus.GaugeValue, float64(status.LastSuccess.Unix()), name)
			output <- prometheus.MustNewConstMetric(exporter.age, prometheus.GaugeValue, max(0, now.Sub(status.LastSuccess).Seconds()), name)
		}
	}
	output <- prometheus.MustNewConstMetric(exporter.buildInfo, prometheus.GaugeValue, 1, exporter.build.Version, exporter.build.Revision, exporter.build.GoVersion)
	for _, event := range exporter.events {
		event.Collect(output)
	}
}

// ObserveRefresh records one collector attempt.
func (exporter *Exporter) ObserveRefresh(name, status string, duration time.Duration) {
	if !exporter.isKnown(name) {
		return
	}
	exporter.refresh.WithLabelValues(name, bounded(status, "success", "error", "canceled")).Inc()
	exporter.refreshDuration.WithLabelValues(name).Observe(seconds(duration))
}

// ObserveRequest records one Cost Explorer API request.
func (exporter *Exporter) ObserveRequest(operation, status string, duration time.Duration) {
	operation = bounded(operation, "GetCostAndUsage", "GetCostForecast")
	exporter.requests.WithLabelValues(operation, bounded(status, "success", "error", "canceled", "throttle")).Inc()
	exporter.requestDuration.WithLabelValues(operation).Observe(seconds(duration))
}

// ObserveRetry records one Cost Explorer SDK retry.
func (exporter *Exporter) ObserveRetry(operation, reason string) {
	exporter.retries.WithLabelValues(
		bounded(operation, "GetCostAndUsage", "GetCostForecast"),
		boundedDefault(reason, "other", "throttle", "timeout", "other"),
	).Inc()
}

// ObserveSkipped records one scheduler single-flight skip.
func (exporter *Exporter) ObserveSkipped(name, reason string) {
	if exporter.isKnown(name) {
		exporter.skipped.WithLabelValues(name, bounded(reason, "single_flight")).Inc()
	}
}

// ObserveOverflow records dimension values folded into an overflow series.
func (exporter *Exporter) ObserveOverflow(dimension string, count int) {
	if count > 0 {
		exporter.overflow.WithLabelValues(bounded(dimension, "service", "region", "account")).Add(float64(count))
	}
}

// ObservePaginationPage records one successfully read Cost Explorer page.
func (exporter *Exporter) ObservePaginationPage(operation string) {
	exporter.pagination.WithLabelValues(bounded(operation, "GetCostAndUsage", "GetCostForecast")).Inc()
}

// ObserveCachePublishError records a cache publish or failure-record error.
func (exporter *Exporter) ObserveCachePublishError(collector, operation string) {
	if !exporter.isKnown(collector) {
		return
	}
	exporter.cachePublishErrors.WithLabelValues(collector, bounded(operation, "publish", "record_failure")).Inc()
}

// ObserveSchedulerShutdownTimeout records one scheduler shutdown timeout.
func (exporter *Exporter) ObserveSchedulerShutdownTimeout() {
	exporter.shutdownTimeouts.Inc()
}

func (exporter *Exporter) isKnown(name string) bool { _, exists := exporter.known[name]; return exists }
func bounded(value string, allowed ...string) string {
	return boundedDefault(value, "unknown", allowed...)
}
func boundedDefault(value, fallback string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return fallback
}
func seconds(duration time.Duration) float64 { return max(0, duration.Seconds()) }
func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
func selfDesc(name, help string, labels []string) *prometheus.Desc {
	return prometheus.NewDesc("aws_cost_exporter_"+name, help, labels, nil)
}
func counter(name, help string, labels []string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aws_cost_exporter_" + name, Help: help}, labels)
}
func histogram(name, help string, labels []string, buckets []float64) *prometheus.HistogramVec {
	return prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "aws_cost_exporter_" + name, Help: help, Buckets: buckets}, labels)
}

var _ prometheus.Collector = (*Exporter)(nil)
