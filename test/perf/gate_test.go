package perf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/sakuya1998/aws-cost-exporter/internal/app"
	basecollector "github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/collector/account"
	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/metrics"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

func TestAccountCollectorSeriesBudgetForFixtureSizes(t *testing.T) {
	reference := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	day, month := cost.DayContaining(reference), cost.MonthContaining(reference)
	mtd, _ := cost.NewPeriod(month.Start(), day.End())
	for _, count := range []int{1, 100, 1001} {
		t.Run(fmt.Sprintf("%d", count), func(t *testing.T) {
			subject, _ := account.New(&budgetReader{count: count}, nil, 1000, basecollector.DefaultOverflowLabel)
			snapshot, err := subject.Collect(context.Background(), reference)
			if err != nil {
				t.Fatal(err)
			}
			for _, window := range []cost.Window{cost.WindowDaily, cost.WindowMonthToDate} {
				series := 0
				for _, value := range snapshot.Costs() {
					if value.Window == window {
						series++
					}
				}
				if series != min(count, 1000) {
					t.Fatalf("%s series=%d", window, series)
				}
			}
			want := sumAmounts(accountCosts(count, cost.WindowDaily, day)) + sumAmounts(accountCosts(count, cost.WindowMonthToDate, mtd))
			if sumAmounts(snapshot.Costs()) != want {
				t.Fatal("conserved total mismatch")
			}
		})
	}
}

func TestScrapeLatencyBaselineUnderFifteenSeconds(t *testing.T) {
	start := time.Now()
	if _, err := testutil.GatherAndCount(newBenchRegistry(t)); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Fatalf("scrape latency=%s", elapsed)
	}
}

func BenchmarkMetricsExposition1000Series(b *testing.B) {
	registry := newBenchRegistry(b)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := testutil.GatherAndCount(registry); err != nil {
			b.Fatal(err)
		}
	}
}

// Assumes unpaginated responses (1 page per query). Paginated deployments exceed 8+1.
func TestStartupRefreshMatchesQueryPaginationBudget(t *testing.T) {
	var usage, forecast atomic.Int32
	baseURL := runPerfExporter(t, budgetHandler(t, &usage, &forecast), true)
	awaitPerfHTTP(t, baseURL+"/metrics", func(code int, body string) bool {
		return code == http.StatusOK &&
			strings.Contains(body, "aws_cost_exporter_aws_api_requests_total{operation=\"GetCostAndUsage\",status=\"success\"} 8\n") &&
			strings.Contains(body, "aws_cost_exporter_aws_api_requests_total{operation=\"GetCostForecast\",status=\"success\"} 1\n")
	})
	if usage.Load() != 8 || forecast.Load() != 1 {
		t.Fatalf("usage=%d forecast=%d", usage.Load(), forecast.Load())
	}
}

func TestMetricsScrapeDoesNotCallAWS(t *testing.T) {
	var calls atomic.Int32
	baseURL := runPerfExporter(t, func(_ http.ResponseWriter, _ *http.Request) { calls.Add(1) }, true)
	awaitPerfHTTP(t, baseURL+"/metrics", func(code int, _ string) bool { return code == http.StatusOK })
	before := calls.Load()
	awaitPerfHTTP(t, baseURL+"/metrics", func(code int, _ string) bool { return code == http.StatusOK })
	if calls.Load() != before {
		t.Fatalf("scrape aws calls before=%d after=%d", before, calls.Load())
	}
}

type budgetRequest struct {
	TimePeriod struct{ Start, End string }
	GroupBy    []struct{ Key string }
}

type budgetReader struct{ count int }

func (reader *budgetReader) ReadCosts(_ context.Context, query ports.CostQuery) ([]cost.Cost, error) {
	return accountCosts(reader.count, query.Window, query.Period), nil
}

type benchStore struct {
	snapshot cost.Snapshot
	statuses map[string]ports.CollectorStatus
}

func (store *benchStore) Snapshot() cost.Snapshot { return store.snapshot }
func (store *benchStore) Load() ports.SnapshotView {
	return ports.SnapshotView{Snapshot: store.snapshot, Collectors: store.statuses}
}

type fixedClock struct{ instant time.Time }

func (clock fixedClock) Now() time.Time { return clock.instant }

func newBenchRegistry(tb testing.TB) *prometheus.Registry {
	tb.Helper()
	store := &benchStore{snapshot: accountSeriesSnapshot(tb, 1000), statuses: map[string]ports.CollectorStatus{
		"total": {Up: true, Series: 2}, "account": {Up: true, Series: 2000}, "forecast": {Up: true, Series: 3},
	}}
	registry := prometheus.NewRegistry()
	costCollector, _ := metrics.NewCostCollector(store)
	exporter, _ := metrics.NewExporter(store, fixedClock{time.Unix(1_700_000_000, 0)}, version.Info{Version: "bench"}, []string{"total", "account", "forecast"})
	registry.MustRegister(costCollector, exporter)
	return registry
}

func accountCosts(count int, window cost.Window, period cost.Period) []cost.Cost {
	costs := make([]cost.Cost, 0, count)
	for index := range count {
		amount, _ := cost.NewMoney(float64(index+1), "USD")
		dimension, _ := cost.NewDimension(cost.DimensionAccount, fmt.Sprintf("%012d", index+1))
		costs = append(costs, cost.Cost{Window: window, Period: period, Dimension: dimension, Amount: amount})
	}
	return costs
}

func sumAmounts(values []cost.Cost) float64 {
	total := 0.0
	for _, value := range values {
		total += value.Amount.Amount()
	}
	return total
}

func accountSeriesSnapshot(tb testing.TB, perWindow int) cost.Snapshot {
	tb.Helper()
	reference := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	day, month := cost.DayContaining(reference), cost.MonthContaining(reference)
	mtd, err := cost.NewPeriod(month.Start(), day.End())
	if err != nil {
		tb.Fatal(err)
	}
	var costs []cost.Cost
	for index := range perWindow {
		for _, item := range []struct {
			window cost.Window
			period cost.Period
		}{{cost.WindowDaily, day}, {cost.WindowMonthToDate, mtd}} {
			amount, _ := cost.NewMoney(float64(index+1), "USD")
			dimension, _ := cost.NewDimension(cost.DimensionAccount, fmt.Sprintf("%012d", index+1))
			costs = append(costs, cost.Cost{Window: item.window, Period: item.period, Dimension: dimension, Amount: amount})
		}
	}
	for _, item := range []struct {
		window cost.Window
		period cost.Period
	}{{cost.WindowDaily, day}, {cost.WindowMonthToDate, mtd}} {
		amount, _ := cost.NewMoney(999, "USD")
		dimension, _ := cost.NewDimension(cost.DimensionTotal, "")
		costs = append(costs, cost.Cost{Window: item.window, Period: item.period, Dimension: dimension, Amount: amount})
	}
	mean, _ := cost.NewMoney(500, "USD")
	lower, _ := cost.NewMoney(400, "USD")
	upper, _ := cost.NewMoney(600, "USD")
	forecastPeriod, _ := cost.NewPeriod(day.End(), month.End())
	return cost.NewSnapshot(costs, []cost.Forecast{{Period: forecastPeriod, Mean: mean, LowerBound: lower, UpperBound: upper}})
}

func budgetHandler(t *testing.T, usage, forecast *atomic.Int32) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if strings.HasSuffix(request.Header.Get("X-Amz-Target"), ".GetCostForecast") {
			forecast.Add(1)
			var input budgetRequest
			_ = json.Unmarshal(body, &input)
			writer.Header().Set("Content-Type", "application/x-amz-json-1.1")
			_, _ = fmt.Fprintf(writer, `{"Total":{"Amount":"100","Unit":"USD"},"ForecastResultsByTime":[{"TimePeriod":{"Start":"%s","End":"%s"},"MeanValue":"100","PredictionIntervalLowerBound":"80","PredictionIntervalUpperBound":"120"}]}`, input.TimePeriod.Start, input.TimePeriod.End)
			return
		}
		usage.Add(1)
		var input budgetRequest
		_ = json.Unmarshal(body, &input)
		if len(input.GroupBy) == 0 {
			writePerfFixture(t, writer, "total.json", map[string]string{"START": input.TimePeriod.Start, "END": input.TimePeriod.End, "AMOUNT": "9"})
			return
		}
		key := "AmazonEC2"
		switch input.GroupBy[0].Key {
		case "LINKED_ACCOUNT":
			key = "123456789012"
		case "REGION":
			key = "us-east-1"
		}
		writePerfFixture(t, writer, "grouped.json", map[string]string{"START": input.TimePeriod.Start, "END": input.TimePeriod.End, "KEY": key, "AMOUNT": "1", "NEXT": ""})
	}
}

func runPerfExporter(t *testing.T, handler http.HandlerFunc, allCollectors bool) string {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "perf")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "perf")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	fakeAWS := httptest.NewServer(handler)
	t.Cleanup(fakeAWS.Close)
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	address := listener.Addr().String()
	_ = listener.Close()
	value := config.Default()
	value.Server.ListenAddress, value.Server.ShutdownTimeout = address, time.Second
	value.AWS.EndpointURL, value.AWS.RequestTimeout = fakeAWS.URL, time.Second
	value.AWS.Retry.MaxAttempts, value.AWS.Retry.BaseDelay, value.AWS.Retry.MaxBackoff = 1, time.Millisecond, time.Millisecond
	value.AWS.RateLimit.RequestsPerSecond, value.AWS.RateLimit.Burst = 1000, 10
	value.CostExplorer.StartupRefresh, value.CostExplorer.JitterRatio = true, 0
	value.Telemetry.IncludeGoCollector, value.Telemetry.IncludeProcessCollector = false, false
	if allCollectors {
		value.CostExplorer.Collectors.Total, value.CostExplorer.Collectors.Service = true, true
		value.CostExplorer.Collectors.Region, value.CostExplorer.Collectors.Account = true, true
		value.CostExplorer.Forecast.Enabled = true
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, value, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	t.Cleanup(func() { cancel(); <-done })
	awaitPerfHTTP(t, "http://"+address+"/healthz", func(code int, _ string) bool { return code == http.StatusOK })
	return "http://" + address
}

func writePerfFixture(t *testing.T, writer http.ResponseWriter, name string, replacements map[string]string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for key, value := range replacements {
		body = strings.ReplaceAll(body, "{{"+key+"}}", value)
	}
	writer.Header().Set("Content-Type", "application/x-amz-json-1.1")
	_, _ = fmt.Fprint(writer, body)
}

func awaitPerfHTTP(t *testing.T, url string, accept func(int, string) bool) {
	t.Helper()
	client := &http.Client{Timeout: 200 * time.Millisecond}
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Get(url)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if accept(response.StatusCode, string(body)) {
			return
		}
	}
	t.Fatalf("condition not met for %s", url)
}
