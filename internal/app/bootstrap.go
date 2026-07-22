package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promcollectors "github.com/prometheus/client_golang/prometheus/collectors"

	athenaapi "github.com/sakuya1998/aws-cost-exporter/internal/aws/athena"
	budgetapi "github.com/sakuya1998/aws-cost-exporter/internal/aws/budgets"
	"github.com/sakuya1998/aws-cost-exporter/internal/aws/clientfactory"
	ce "github.com/sakuya1998/aws-cost-exporter/internal/aws/costexplorer"
	organizationsapi "github.com/sakuya1998/aws-cost-exporter/internal/aws/organizations"
	"github.com/sakuya1998/aws-cost-exporter/internal/cache/memory"
	basecollector "github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/collector/account"
	anomalycollector "github.com/sakuya1998/aws-cost-exporter/internal/collector/anomaly"
	budgetcollector "github.com/sakuya1998/aws-cost-exporter/internal/collector/budget"
	commitmentcollector "github.com/sakuya1998/aws-cost-exporter/internal/collector/commitment"
	curcollector "github.com/sakuya1998/aws-cost-exporter/internal/collector/cur"
	"github.com/sakuya1998/aws-cost-exporter/internal/collector/forecast"
	organizationcollector "github.com/sakuya1998/aws-cost-exporter/internal/collector/organizationmeta"
	"github.com/sakuya1998/aws-cost-exporter/internal/collector/region"
	"github.com/sakuya1998/aws-cost-exporter/internal/collector/service"
	tagcollector "github.com/sakuya1998/aws-cost-exporter/internal/collector/tag"
	"github.com/sakuya1998/aws-cost-exporter/internal/collector/total"
	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/tagcost"
	"github.com/sakuya1998/aws-cost-exporter/internal/httpserver"
	appmetrics "github.com/sakuya1998/aws-cost-exporter/internal/metrics"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
	"github.com/sakuya1998/aws-cost-exporter/internal/scheduler"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

// Run constructs all production adapters and blocks until shutdown.
func Run(ctx context.Context, value config.Config, logger *slog.Logger) error {
	ids := configuredCollectorIDs(value)
	if len(ids) == 0 {
		return errors.New("no collectors enabled")
	}
	clock := systemClock{}
	store, err := memory.New(
		clock, value.Cache.FreshnessTTL, value.Cache.StaleAfter,
		memory.WithOrganizationPolicies(organizationPolicies(value)),
	)
	if err != nil {
		return err
	}
	telemetry, err := appmetrics.NewExporter(store, clock, version.Current(), ids)
	if err != nil {
		return err
	}
	awsFactory, err := clientfactory.New(ctx, value.AWS, telemetry)
	if err != nil {
		return err
	}
	jobs, required, err := buildJobs(value, awsFactory, telemetry)
	if err != nil {
		return err
	}

	businessMetrics, err := appmetrics.NewCostCollector(store)
	if err != nil {
		return err
	}
	prometheusRegistry := prometheus.NewRegistry()
	prometheusCollectors := []prometheus.Collector{businessMetrics, telemetry}
	if value.Telemetry.IncludeGoCollector {
		prometheusCollectors = append(prometheusCollectors, promcollectors.NewGoCollector())
	}
	if value.Telemetry.IncludeProcessCollector {
		prometheusCollectors = append(prometheusCollectors, promcollectors.NewProcessCollector(promcollectors.ProcessCollectorOpts{}))
	}
	for _, collector := range prometheusCollectors {
		if err := prometheusRegistry.Register(collector); err != nil {
			return err
		}
	}

	runner, err := scheduler.NewJobs(jobs, store, clock, nil, scheduler.Config{
		JitterRatio:    value.Collection.JitterRatio,
		MaxConcurrency: value.Collection.MaxConcurrency,
		Backoff: scheduler.BackoffConfig{
			MaxAttempts: value.Collection.FailureBackoff.MaxAttempts,
			Initial:     value.Collection.FailureBackoff.Initial,
			Max:         value.Collection.FailureBackoff.Max,
			Multiplier:  value.Collection.FailureBackoff.Multiplier,
		},
		Observer: telemetry, Logger: logger,
	})
	if err != nil {
		return err
	}
	server, err := httpserver.New(value.Server, prometheusRegistry, store, required, version.Current())
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", value.Server.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info("aws-cost-exporter started", "address", listener.Addr().String(), "targets", len(value.Targets))
	for _, target := range value.Targets {
		if unfilteredGroupedCollectors(value, target) {
			logger.Warn("grouped collectors enabled without Cost Explorer filters; queries may return many billable pages", "target", target.Name)
		}
	}
	return RunServices(ctx, runner, server, listener, value.Server.ShutdownTimeout, logger, telemetry.ObserveSchedulerShutdownTimeout)
}

func buildJobs(value config.Config, factory *clientfactory.Factory, telemetry *appmetrics.Exporter) ([]scheduler.Job, []identity.CollectorID, error) {
	jobs := make([]scheduler.Job, 0, len(configuredCollectorIDs(value)))
	required := make([]identity.CollectorID, 0)
	for _, targetConfig := range value.Targets {
		target := identity.TargetID(targetConfig.Name)
		clients, err := factory.ForTarget(targetConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("construct target %s AWS clients: %w", target, err)
		}
		if targetConfig.CostExplorer.Enabled {
			instrumented, instrumentErr := ce.NewInstrumented(target, clients.CostExplorer, telemetry, clients.Retryer)
			if instrumentErr != nil {
				return nil, nil, instrumentErr
			}
			usage, usageErr := ce.NewUsageAdapterForTarget(target, instrumented, value.Collection.CostExplorer.MaxPages, "UnblendedCost", telemetry)
			if usageErr != nil {
				return nil, nil, usageErr
			}
			forecastReader, forecastErr := ce.NewForecastAdapter(instrumented)
			if forecastErr != nil {
				return nil, nil, forecastErr
			}
			reader := filteredReader{UsageAdapter: usage, ForecastAdapter: forecastReader, filters: targetConfig.CostExplorer.Filters, bases: configuredCostBases(value.Collection.CostExplorer.CostBases)}
			costCollectors, collectorErr := buildCostCollectors(target, reader, targetConfig, value, telemetry)
			if collectorErr != nil {
				return nil, nil, collectorErr
			}
			for _, collector := range costCollectors {
				jobs = append(jobs, scheduler.Job{Collector: verifiedCollector{Collector: collector, verifier: clients.Verifier}, Interval: value.Collection.RefreshInterval, StartupRefresh: value.Collection.StartupRefresh})
				if targetConfig.Required {
					required = append(required, collector.ID())
				}
			}
			if targetConfig.Tags.Enabled {
				tagUsage, tagUsageErr := ce.NewUsageAdapterForTarget(target, instrumented, value.Collection.Tags.MaxPages, "UnblendedCost", telemetry)
				if tagUsageErr != nil {
					return nil, nil, tagUsageErr
				}
				tagReader := filteredReader{UsageAdapter: tagUsage, ForecastAdapter: forecastReader, filters: targetConfig.CostExplorer.Filters, bases: configuredCostBases(value.Collection.CostExplorer.CostBases)}
				collector, collectorErr := tagcollector.New(target, tagReader, configuredCostBases(value.Collection.CostExplorer.CostBases), targetConfig.Tags.Keys, value.Collection.Tags.SeriesLimit, value.Collection.CostExplorer.Dimensions.OverflowLabel, targetOverflowObserver{target: target, telemetry: telemetry})
				if collectorErr != nil {
					return nil, nil, collectorErr
				}
				jobs = append(jobs, scheduler.Job{Collector: verifiedCollector{Collector: collector, verifier: clients.Verifier}, Interval: value.Collection.Tags.RefreshInterval, StartupRefresh: value.Collection.StartupRefresh})
			}
		}
		if targetConfig.Organizations.Enabled {
			reader, readerErr := organizationsapi.New(
				target, clients.Organizations, value.Collection.Organizations.MaxPages,
				organizationsapi.Policy{
					AccountIDs:  targetConfig.Organizations.AccountIDs,
					SeriesLimit: value.Collection.Organizations.SeriesLimit,
				},
				telemetry, clients.Retryer,
			)
			if readerErr != nil {
				return nil, nil, readerErr
			}
			collector, collectorErr := organizationcollector.New(target, reader)
			if collectorErr != nil {
				return nil, nil, collectorErr
			}
			jobs = append(jobs, scheduler.Job{Collector: verifiedCollector{Collector: collector, verifier: clients.Verifier}, Interval: value.Collection.Organizations.RefreshInterval, StartupRefresh: value.Collection.StartupRefresh})
		}
		if targetConfig.Budgets.Enabled {
			reader, readerErr := budgetapi.New(target, targetConfig.AccountID, clients.Budgets, value.Collection.Budgets.MaxPages, targetConfig.Budgets.Names, telemetry, clients.Retryer)
			if readerErr != nil {
				return nil, nil, readerErr
			}
			collector, collectorErr := budgetcollector.New(target, reader)
			if collectorErr != nil {
				return nil, nil, collectorErr
			}
			jobs = append(jobs, scheduler.Job{Collector: verifiedCollector{Collector: collector, verifier: clients.Verifier}, Interval: value.Collection.Budgets.RefreshInterval, StartupRefresh: value.Collection.StartupRefresh})
		}
		if targetConfig.Commitments.Enabled {
			reader, readerErr := ce.NewCommitmentReader(target, clients.CostExplorer, value.Collection.Commitments.MaxPages, telemetry, clients.Retryer)
			if readerErr != nil {
				return nil, nil, readerErr
			}
			collector, collectorErr := commitmentcollector.New(target, reader, value.Collection.Commitments.SeriesLimit)
			if collectorErr != nil {
				return nil, nil, collectorErr
			}
			jobs = append(jobs, scheduler.Job{Collector: verifiedCollector{Collector: collector, verifier: clients.Verifier}, Interval: value.Collection.Commitments.RefreshInterval, StartupRefresh: value.Collection.StartupRefresh})
		}
		if targetConfig.Anomalies.Enabled {
			reader, readerErr := ce.NewAnomalyReader(target, clients.CostExplorer, value.Collection.Anomalies.MaxPages, telemetry, clients.Retryer)
			if readerErr != nil {
				return nil, nil, readerErr
			}
			collector, collectorErr := anomalycollector.New(target, reader, value.Collection.Anomalies.SeriesLimit)
			if collectorErr != nil {
				return nil, nil, collectorErr
			}
			jobs = append(jobs, scheduler.Job{Collector: verifiedCollector{Collector: collector, verifier: clients.Verifier}, Interval: value.Collection.Anomalies.RefreshInterval, StartupRefresh: value.Collection.StartupRefresh})
		}
		if targetConfig.CUR.Enabled {
			reader, readerErr := athenaapi.NewReader(target, clients.Athena, targetConfig.CUR, value.Collection.CUR.MaxPages, value.Collection.CUR.MaxRows, telemetry, clients.Retryer)
			if readerErr != nil {
				return nil, nil, readerErr
			}
			collector, collectorErr := curcollector.New(target, reader, configuredCostBases(value.Collection.CostExplorer.CostBases), targetConfig.Tags.Enabled, value.Collection.CUR.SeriesLimit, value.Collection.Tags.SeriesLimit, targetConfig.Tags.Keys, value.Collection.CostExplorer.Dimensions.OverflowLabel, targetOverflowObserver{target: target, telemetry: telemetry})
			if collectorErr != nil {
				return nil, nil, collectorErr
			}
			jobs = append(jobs, scheduler.Job{Collector: verifiedCollector{Collector: collector, verifier: clients.Verifier}, Interval: value.Collection.CUR.RefreshInterval, StartupRefresh: value.Collection.StartupRefresh})
		}
	}
	return jobs, required, nil
}

type verifiedCollector struct {
	basecollector.Collector
	verifier clientfactory.Verifier
}

func (collector verifiedCollector) Collect(ctx context.Context, reference time.Time) (snapshot.PartialSnapshot, error) {
	if err := collector.verifier.Verify(ctx); err != nil {
		return snapshot.PartialSnapshot{}, err
	}
	return collector.Collector.Collect(ctx, reference)
}

func buildCostCollectors(target identity.TargetID, reader filteredReader, targetConfig config.TargetConfig, value config.Config, telemetry *appmetrics.Exporter) ([]basecollector.Collector, error) {
	settings := value.Collection.CostExplorer
	overflow := targetOverflowObserver{target: target, telemetry: telemetry}
	result := make([]basecollector.Collector, 0, 5)
	constructors := []struct {
		enabled bool
		build   func() (basecollector.Collector, error)
	}{
		{settings.Collectors.Total, func() (basecollector.Collector, error) { return total.NewForTarget(target, reader) }},
		{settings.Collectors.Service, func() (basecollector.Collector, error) {
			return service.NewForTarget(target, reader, settings.Dimensions.SeriesLimit, settings.Dimensions.OverflowLabel, overflow)
		}},
		{settings.Collectors.Region, func() (basecollector.Collector, error) {
			return region.NewForTarget(target, reader, settings.Dimensions.SeriesLimit, settings.Dimensions.OverflowLabel, overflow)
		}},
		{settings.Collectors.Account, func() (basecollector.Collector, error) {
			return account.NewForTarget(target, reader, targetConfig.CostExplorer.Filters.LinkedAccountIDs, settings.Dimensions.SeriesLimit, settings.Dimensions.OverflowLabel, overflow)
		}},
		{settings.Collectors.Forecast, func() (basecollector.Collector, error) {
			return forecast.NewForTarget(target, reader, settings.PredictionInterval)
		}},
	}
	for _, constructor := range constructors {
		if !constructor.enabled {
			continue
		}
		collector, err := constructor.build()
		if err != nil {
			return nil, err
		}
		result = append(result, collector)
	}
	return result, nil
}

func configuredCollectorIDs(value config.Config) []identity.CollectorID {
	ids := make([]identity.CollectorID, 0, len(value.Targets)*10)
	for _, targetConfig := range value.Targets {
		target := identity.TargetID(targetConfig.Name)
		if targetConfig.CostExplorer.Enabled {
			collectors := value.Collection.CostExplorer.Collectors
			for _, item := range []struct {
				name    string
				enabled bool
			}{
				{total.Name, collectors.Total}, {service.Name, collectors.Service}, {region.Name, collectors.Region},
				{account.Name, collectors.Account}, {forecast.Name, collectors.Forecast},
			} {
				if item.enabled {
					ids = append(ids, identity.CollectorID{Target: target, Name: item.name})
				}
			}
			if targetConfig.Tags.Enabled {
				ids = append(ids, identity.CollectorID{Target: target, Name: tagcollector.Name})
			}
		}
		if targetConfig.Organizations.Enabled {
			ids = append(ids, identity.CollectorID{Target: target, Name: organizationcollector.Name})
		}
		if targetConfig.Budgets.Enabled {
			ids = append(ids, identity.CollectorID{Target: target, Name: budgetcollector.Name})
		}
		if targetConfig.Commitments.Enabled {
			ids = append(ids, identity.CollectorID{Target: target, Name: commitmentcollector.Name})
		}
		if targetConfig.Anomalies.Enabled {
			ids = append(ids, identity.CollectorID{Target: target, Name: anomalycollector.Name})
		}
		if targetConfig.CUR.Enabled {
			ids = append(ids, identity.CollectorID{Target: target, Name: curcollector.Name})
		}
	}
	return ids
}

func organizationPolicies(value config.Config) map[identity.TargetID]memory.OrganizationPolicy {
	result := make(map[identity.TargetID]memory.OrganizationPolicy)
	for _, target := range value.Targets {
		if target.Organizations.Enabled {
			result[identity.TargetID(target.Name)] = memory.OrganizationPolicy{AccountIDs: target.Organizations.AccountIDs, SeriesLimit: value.Collection.Organizations.SeriesLimit}
		}
	}
	return result
}

type targetOverflowObserver struct {
	target    identity.TargetID
	telemetry *appmetrics.Exporter
}

func (observer targetOverflowObserver) ObserveOverflow(dimension string, count int) {
	observer.telemetry.ObserveOverflow(observer.target, dimension, count)
}

type filteredReader struct {
	*ce.UsageAdapter
	*ce.ForecastAdapter
	filters config.FiltersConfig
	bases   []cost.Basis
}

func (reader filteredReader) ReadCosts(ctx context.Context, query ports.CostQuery) ([]cost.Cost, error) {
	query.LinkedAccountIDs = append([]string(nil), reader.filters.LinkedAccountIDs...)
	query.Services = append([]string(nil), reader.filters.Services...)
	query.Regions = append([]string(nil), reader.filters.Regions...)
	bases := reader.bases
	if len(bases) == 0 {
		bases = []cost.Basis{cost.BasisUnblended}
	}
	var values []cost.Cost
	for _, basis := range bases {
		current := query
		current.Basis = basis
		result, err := reader.UsageAdapter.ReadCosts(ctx, current)
		if err != nil {
			return nil, err
		}
		values = append(values, result...)
	}
	return values, nil
}
func (reader filteredReader) ReadForecast(ctx context.Context, query ports.ForecastQuery) (cost.Forecast, error) {
	query.LinkedAccountIDs = append([]string(nil), reader.filters.LinkedAccountIDs...)
	query.Services = append([]string(nil), reader.filters.Services...)
	query.Regions = append([]string(nil), reader.filters.Regions...)
	if len(reader.bases) > 0 {
		query.Basis = reader.bases[0]
	} else {
		query.Basis = cost.BasisUnblended
	}
	return reader.ForecastAdapter.ReadForecast(ctx, query)
}

func (reader filteredReader) ReadTagCosts(ctx context.Context, query ports.CostQuery, tagKey string) ([]tagcost.Cost, error) {
	query.LinkedAccountIDs = append([]string(nil), reader.filters.LinkedAccountIDs...)
	query.Services = append([]string(nil), reader.filters.Services...)
	query.Regions = append([]string(nil), reader.filters.Regions...)
	return reader.UsageAdapter.ReadTagCosts(ctx, query, tagKey)
}

func configuredCostBases(values []string) []cost.Basis {
	result := make([]cost.Basis, 0, len(values))
	for _, value := range values {
		result = append(result, cost.Basis(value))
	}
	return result
}

func unfilteredGroupedCollectors(value config.Config, target config.TargetConfig) bool {
	collectors := value.Collection.CostExplorer.Collectors
	filters := target.CostExplorer.Filters
	return target.CostExplorer.Enabled && (collectors.Service || collectors.Region || collectors.Account) && len(filters.LinkedAccountIDs) == 0 && len(filters.Services) == 0 && len(filters.Regions) == 0
}

type schedulerService interface{ Run(context.Context) }

// RunServices coordinates the scheduler and HTTP server until shutdown.
func RunServices(ctx context.Context, runner schedulerService, server *httpserver.Server, listener net.Listener, schedulerShutdownTimeout time.Duration, logger *slog.Logger, onSchedulerShutdownTimeout func()) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	schedulerDone, serverDone := make(chan struct{}), make(chan error, 1)
	go func() { runner.Run(runCtx); close(schedulerDone) }()
	go func() { serverDone <- server.Serve(listener) }()
	select {
	case err := <-serverDone:
		cancel()
		waitForScheduler(schedulerDone, schedulerShutdownTimeout, logger, onSchedulerShutdownTimeout)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		cancel()
		shutdownErr := server.Shutdown(context.Background())
		waitForScheduler(schedulerDone, schedulerShutdownTimeout, logger, onSchedulerShutdownTimeout)
		serveErr := <-serverDone
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		return errors.Join(shutdownErr, serveErr)
	}
}

func waitForScheduler(done <-chan struct{}, timeout time.Duration, logger *slog.Logger, onTimeout func()) {
	if timeout <= 0 {
		<-done
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		if logger != nil {
			logger.Warn("scheduler did not stop before shutdown timeout", "timeout", timeout)
		}
		if onTimeout != nil {
			onTimeout()
		}
	}
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }
func (systemClock) NewTimer(delay time.Duration) scheduler.Timer {
	return scheduler.NewSystemTimer(delay)
}

var _ scheduler.Observer = (*appmetrics.Exporter)(nil)
