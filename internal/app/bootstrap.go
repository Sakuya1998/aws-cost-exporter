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

	ce "github.com/sakuya1998/aws-cost-exporter/internal/aws/costexplorer"
	"github.com/sakuya1998/aws-cost-exporter/internal/cache/memory"
	basecollector "github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/collector/account"
	"github.com/sakuya1998/aws-cost-exporter/internal/collector/forecast"
	"github.com/sakuya1998/aws-cost-exporter/internal/collector/region"
	"github.com/sakuya1998/aws-cost-exporter/internal/collector/service"
	"github.com/sakuya1998/aws-cost-exporter/internal/collector/total"
	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/httpserver"
	appmetrics "github.com/sakuya1998/aws-cost-exporter/internal/metrics"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
	"github.com/sakuya1998/aws-cost-exporter/internal/scheduler"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

// Run constructs all production adapters and blocks until shutdown.
func Run(ctx context.Context, value config.Config, logger *slog.Logger) error {
	names := enabledCollectors(value)
	if len(names) == 0 {
		return errors.New("no Cost Explorer collectors enabled")
	}
	clock := systemClock{}
	store, err := memory.New(clock, value.Cache.FreshnessTTL, value.Cache.StaleAfter)
	if err != nil {
		return err
	}
	telemetry, err := appmetrics.NewExporter(store, clock, version.Current(), names)
	if err != nil {
		return err
	}
	raw, err := ce.New(ctx, value.AWS)
	if err != nil {
		return err
	}
	instrumented, err := ce.NewInstrumented(raw, value.AWS.RateLimit, telemetry)
	if err != nil {
		return err
	}
	usage, err := ce.NewUsageAdapter(instrumented, value.CostExplorer.MaxPages, telemetry)
	if err != nil {
		return err
	}
	forecastReader, err := ce.NewForecastAdapter(instrumented)
	if err != nil {
		return err
	}
	reader := filteredReader{
		UsageAdapter: usage, ForecastAdapter: forecastReader,
		filters: value.CostExplorer.Filters,
	}
	registry := basecollector.NewRegistry()
	factories := []struct {
		name string
		new  basecollector.Factory
	}{
		{total.Name, func() (basecollector.Collector, error) { return total.New(reader) }},
		{service.Name, func() (basecollector.Collector, error) {
			return service.New(reader, value.CostExplorer.Dimensions.SeriesLimit, telemetry)
		}},
		{region.Name, func() (basecollector.Collector, error) {
			return region.New(reader, value.CostExplorer.Dimensions.SeriesLimit, telemetry)
		}},
		{account.Name, func() (basecollector.Collector, error) {
			return account.New(reader, value.CostExplorer.Filters.LinkedAccountIDs, value.CostExplorer.Dimensions.SeriesLimit, telemetry)
		}},
		{forecast.Name, func() (basecollector.Collector, error) {
			return forecast.New(reader, value.CostExplorer.Forecast.PredictionInterval)
		}},
	}
	for _, registration := range factories {
		if err := registry.Register(registration.name, registration.new); err != nil {
			return err
		}
	}
	collectors, err := registry.Build(names)
	if err != nil {
		return err
	}
	costMetrics, err := appmetrics.NewCostCollector(store)
	if err != nil {
		return err
	}
	prometheusRegistry := prometheus.NewRegistry()
	prometheusCollectors := []prometheus.Collector{costMetrics, telemetry}
	if value.Telemetry.IncludeGoCollector {
		prometheusCollectors = append(prometheusCollectors, promcollectors.NewGoCollector())
	}
	if value.Telemetry.IncludeProcessCollector {
		prometheusCollectors = append(prometheusCollectors, promcollectors.NewProcessCollector(promcollectors.ProcessCollectorOpts{}))
	}
	for _, metricCollector := range prometheusCollectors {
		if err := prometheusRegistry.Register(metricCollector); err != nil {
			return err
		}
	}
	runner, err := scheduler.New(collectors, store, clock, nil, scheduler.Config{
		Interval: value.CostExplorer.RefreshInterval, StartupRefresh: value.CostExplorer.StartupRefresh,
		JitterRatio: value.CostExplorer.JitterRatio, MaxConcurrency: value.Scheduler.MaxConcurrency,
		Backoff: scheduler.BackoffConfig{
			Initial: value.Scheduler.FailureBackoff.Initial, Max: value.Scheduler.FailureBackoff.Max,
			Multiplier: value.Scheduler.FailureBackoff.Multiplier,
		},
		Observer: telemetry,
		Logger:   logger,
	})
	if err != nil {
		return err
	}
	server, err := httpserver.New(value.Server, prometheusRegistry, store, names, version.Current())
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", value.Server.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info("aws-cost-exporter started", "address", listener.Addr().String())
	return RunServices(ctx, runner, server, listener, value.Server.ShutdownTimeout, logger)
}

type schedulerService interface {
	Run(context.Context)
}

// RunServices coordinates the scheduler and HTTP server until shutdown.
func RunServices(
	ctx context.Context,
	runner schedulerService,
	server *httpserver.Server,
	listener net.Listener,
	schedulerShutdownTimeout time.Duration,
	logger *slog.Logger,
) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	schedulerDone, serverDone := make(chan struct{}), make(chan error, 1)
	go func() { runner.Run(runCtx); close(schedulerDone) }()
	go func() { serverDone <- server.Serve(listener) }()
	select {
	case err := <-serverDone:
		cancel()
		<-schedulerDone
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		cancel()
		shutdownErr := server.Shutdown(context.Background())
		waitForScheduler(runner, schedulerDone, schedulerShutdownTimeout, logger)
		serveErr := <-serverDone
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		return errors.Join(shutdownErr, serveErr)
	}
}

func waitForScheduler(_ schedulerService, done <-chan struct{}, timeout time.Duration, logger *slog.Logger) {
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
	}
}

func enabledCollectors(value config.Config) []string {
	if !value.CostExplorer.Enabled {
		return nil
	}
	names := make([]string, 0, 5)
	for _, item := range []struct {
		name string
		on   bool
	}{{total.Name, value.CostExplorer.Collectors.Total},
		{service.Name, value.CostExplorer.Collectors.Service},
		{region.Name, value.CostExplorer.Collectors.Region},
		{account.Name, value.CostExplorer.Collectors.Account},
		{forecast.Name, value.CostExplorer.Forecast.Enabled}} {
		if item.on {
			names = append(names, item.name)
		}
	}
	return names
}

type filteredReader struct {
	*ce.UsageAdapter
	*ce.ForecastAdapter
	filters config.FiltersConfig
}

func (reader filteredReader) ReadCosts(ctx context.Context, query ports.CostQuery) ([]cost.Cost, error) {
	query.LinkedAccountIDs = append([]string(nil), reader.filters.LinkedAccountIDs...)
	query.Services = append([]string(nil), reader.filters.Services...)
	query.Regions = append([]string(nil), reader.filters.Regions...)
	return reader.UsageAdapter.ReadCosts(ctx, query)
}

func (reader filteredReader) ReadForecast(ctx context.Context, query ports.ForecastQuery) (cost.Forecast, error) {
	query.LinkedAccountIDs = append([]string(nil), reader.filters.LinkedAccountIDs...)
	query.Services = append([]string(nil), reader.filters.Services...)
	query.Regions = append([]string(nil), reader.filters.Regions...)
	return reader.ForecastAdapter.ReadForecast(ctx, query)
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) NewTimer(delay time.Duration) scheduler.Timer {
	return scheduler.NewSystemTimer(delay)
}

var _ ce.Observer = (*appmetrics.Exporter)(nil)
var _ scheduler.Observer = (*appmetrics.Exporter)(nil)
