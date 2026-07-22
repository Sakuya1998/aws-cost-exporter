package metrics

import (
	"errors"
	"sort"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

type StatusReader interface{ Load() ports.SnapshotView }

var ErrInvalidExporter = errors.New("invalid exporter metrics configuration")

type Exporter struct {
	reader StatusReader
	clock  ports.Clock
	ids    []identity.CollectorID
	known  map[identity.CollectorID]struct{}
	build  version.Info

	up, success, attempt, age, series, buildInfo  *prometheus.Desc
	refresh, requests, retries, skipped, overflow *prometheus.CounterVec
	pagination, cachePublishErrors                *prometheus.CounterVec
	shutdownTimeouts                              prometheus.Counter
	refreshDuration, requestDuration              *prometheus.HistogramVec
	events                                        []prometheus.Collector
}

func NewExporter(reader StatusReader, clock ports.Clock, build version.Info, ids []identity.CollectorID) (*Exporter, error) {
	if reader == nil || clock == nil || len(ids) == 0 {
		return nil, ErrInvalidExporter
	}
	known := make(map[identity.CollectorID]struct{}, len(ids))
	for _, id := range ids {
		if !id.Valid() {
			return nil, ErrInvalidExporter
		}
		if _, duplicate := known[id]; duplicate {
			return nil, ErrInvalidExporter
		}
		known[id] = struct{}{}
	}
	ids = append([]identity.CollectorID(nil), ids...)
	sort.Slice(ids, func(left, right int) bool {
		if ids[left].Target != ids[right].Target {
			return ids[left].Target < ids[right].Target
		}
		return ids[left].Name < ids[right].Name
	})
	exporter := &Exporter{reader: reader, clock: clock, ids: ids, known: known, build: build}
	targetCollector := []string{"target", "collector"}
	exporter.up = selfDesc("collector_up", "Whether the target collector's latest attempt succeeded.", targetCollector)
	exporter.success = selfDesc("last_success_timestamp_seconds", "Target collector's latest success Unix timestamp.", targetCollector)
	exporter.attempt = selfDesc("last_attempt_timestamp_seconds", "Target collector's latest attempt Unix timestamp.", targetCollector)
	exporter.age = selfDesc("cache_age_seconds", "Seconds since the target collector last succeeded.", targetCollector)
	exporter.series = selfDesc("snapshot_series", "Current business series owned by the target collector.", targetCollector)
	exporter.buildInfo = selfDesc("build_info", "Build metadata for aws-cost-exporter.", []string{"version", "revision", "go_version"})
	exporter.refresh = counter("refresh_total", "Target collector refresh attempts.", []string{"target", "collector", "status"})
	exporter.requests = counter("aws_api_requests_total", "Logical AWS SDK operations.", []string{"target", "operation", "status"})
	exporter.retries = counter("aws_api_retries_total", "Authorized AWS SDK retry attempts.", []string{"target", "operation", "reason"})
	exporter.skipped = counter("scheduler_skipped_runs_total", "Scheduler runs skipped before execution.", []string{"target", "collector", "reason"})
	exporter.overflow = counter("dimension_overflow_values_total", "Dimension values processed into overflow during collection attempts.", []string{"target", "dimension"})
	exporter.pagination = counter("pagination_pages_total", "AWS API pagination pages read successfully.", []string{"target", "operation"})
	exporter.cachePublishErrors = counter("cache_publish_errors_total", "Cache publish or failure-record errors.", []string{"target", "collector", "operation"})
	exporter.shutdownTimeouts = prometheus.NewCounter(prometheus.CounterOpts{Name: "aws_cost_exporter_scheduler_shutdown_timeouts_total", Help: "Scheduler shutdown waits that exceeded server.shutdown_timeout."})
	exporter.refreshDuration = histogram("refresh_duration_seconds", "Target collector refresh duration.", []string{"target", "collector"}, []float64{1, 5, 10, 30, 60, 120, 300})
	exporter.requestDuration = histogram("aws_api_request_duration_seconds", "Logical AWS SDK operation duration, including rate-limit waits and retries.", []string{"target", "operation"}, []float64{.1, .5, 1, 2, 5, 10, 30, 60})
	exporter.events = []prometheus.Collector{exporter.refresh, exporter.requests, exporter.retries, exporter.skipped, exporter.overflow, exporter.pagination, exporter.cachePublishErrors, exporter.shutdownTimeouts, exporter.refreshDuration, exporter.requestDuration}
	return exporter, nil
}

func (exporter *Exporter) Describe(output chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{exporter.up, exporter.success, exporter.attempt, exporter.age, exporter.series, exporter.buildInfo} {
		output <- desc
	}
	for _, event := range exporter.events {
		event.Describe(output)
	}
}

func (exporter *Exporter) Collect(output chan<- prometheus.Metric) {
	view, now := exporter.reader.Load(), exporter.clock.Now()
	for _, id := range exporter.ids {
		status := view.Collectors[id]
		labels := []string{string(id.Target), id.Name}
		output <- prometheus.MustNewConstMetric(exporter.up, prometheus.GaugeValue, boolFloat(status.Up), labels...)
		output <- prometheus.MustNewConstMetric(exporter.series, prometheus.GaugeValue, float64(status.Series), labels...)
		if !status.LastAttempt.IsZero() {
			output <- prometheus.MustNewConstMetric(exporter.attempt, prometheus.GaugeValue, float64(status.LastAttempt.Unix()), labels...)
		}
		if !status.LastSuccess.IsZero() {
			output <- prometheus.MustNewConstMetric(exporter.success, prometheus.GaugeValue, float64(status.LastSuccess.Unix()), labels...)
			output <- prometheus.MustNewConstMetric(exporter.age, prometheus.GaugeValue, max(0, now.Sub(status.LastSuccess).Seconds()), labels...)
		}
	}
	output <- prometheus.MustNewConstMetric(exporter.buildInfo, prometheus.GaugeValue, 1, exporter.build.Version, exporter.build.Revision, exporter.build.GoVersion)
	for _, event := range exporter.events {
		event.Collect(output)
	}
}

func (exporter *Exporter) ObserveRefresh(id identity.CollectorID, status string, duration time.Duration) {
	if !exporter.isKnown(id) {
		return
	}
	exporter.refresh.WithLabelValues(string(id.Target), id.Name, bounded(status, "success", "error", "canceled")).Inc()
	exporter.refreshDuration.WithLabelValues(string(id.Target), id.Name).Observe(seconds(duration))
}

func (exporter *Exporter) ObserveRequest(target identity.TargetID, operation, status string, duration time.Duration) {
	operation = boundedOperation(operation)
	exporter.requests.WithLabelValues(string(target), operation, bounded(status, "success", "error", "canceled", "throttle")).Inc()
	exporter.requestDuration.WithLabelValues(string(target), operation).Observe(seconds(duration))
}

func (exporter *Exporter) ObserveRetry(target identity.TargetID, operation, reason string) {
	exporter.retries.WithLabelValues(string(target), boundedOperation(operation), boundedDefault(reason, "other", "throttle", "timeout", "other")).Inc()
}

func (exporter *Exporter) ObserveSkipped(id identity.CollectorID, reason string) {
	if exporter.isKnown(id) {
		exporter.skipped.WithLabelValues(string(id.Target), id.Name, bounded(reason, "single_flight")).Inc()
	}
}

func (exporter *Exporter) ObserveOverflow(target identity.TargetID, dimension string, count int) {
	if count > 0 {
		exporter.overflow.WithLabelValues(string(target), bounded(dimension, "service", "region", "account", "tag")).Add(float64(count))
	}
}

func (exporter *Exporter) ObservePaginationPage(target identity.TargetID, operation string) {
	exporter.pagination.WithLabelValues(string(target), boundedOperation(operation)).Inc()
}

func (exporter *Exporter) ObserveCachePublishError(id identity.CollectorID, operation string) {
	if exporter.isKnown(id) {
		exporter.cachePublishErrors.WithLabelValues(string(id.Target), id.Name, bounded(operation, "publish", "record_failure")).Inc()
	}
}

func (exporter *Exporter) ObserveSchedulerShutdownTimeout() { exporter.shutdownTimeouts.Inc() }
func (exporter *Exporter) isKnown(id identity.CollectorID) bool {
	_, exists := exporter.known[id]
	return exists
}
func boundedOperation(value string) string {
	return bounded(value, "AssumeRole", "GetCallerIdentity", "GetCostAndUsage", "GetCostForecast", "ListAccounts", "DescribeOrganization", "DescribeBudgets", "GetSavingsPlansUtilization", "GetSavingsPlansCoverage", "GetReservationUtilization", "GetReservationCoverage", "GetAnomalies", "StartQueryExecution", "GetQueryExecution", "GetQueryResults")
}
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
