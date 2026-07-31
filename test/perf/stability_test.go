package perf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

type stabilityResult struct {
	Version          string  `json:"version"`
	SourceSHA        string  `json:"source_sha"`
	GoVersion        string  `json:"go_version"`
	Kernel           string  `json:"kernel"`
	CPUCount         int     `json:"cpu_count"`
	MemoryLimitBytes int64   `json:"memory_limit_bytes"`
	DurationSeconds  float64 `json:"duration_seconds"`
	Targets          int     `json:"targets"`
	BusinessSeries   int     `json:"business_series"`
	Scrapes          int     `json:"scrapes"`
	Errors           int     `json:"errors"`
	P99Milliseconds  float64 `json:"p99_milliseconds"`
	HeapGrowthBytes  int64   `json:"heap_growth_bytes"`
	RSSGrowthBytes   int64   `json:"rss_growth_bytes"`
	GoroutineGrowth  int     `json:"goroutine_growth"`
	ConnectionGrowth int     `json:"connection_growth"`
	HeapSlopePerHour float64 `json:"heap_slope_bytes_per_hour"`
	RSSSlopePerHour  float64 `json:"rss_slope_bytes_per_hour"`
	GoroutineSlope   float64 `json:"goroutine_slope_per_hour"`
	ConnectionSlope  float64 `json:"connection_slope_per_hour"`
	Passed           bool    `json:"passed"`
}

type resourceSample struct {
	elapsed                 time.Duration
	heap, rss               int64
	goroutines, connections int
}

func TestV1StabilitySoak(t *testing.T) {
	if os.Getenv("AWS_COST_EXPORTER_STABILITY") != "1" {
		t.Skip("set AWS_COST_EXPORTER_STABILITY=1 to run the v1 stability soak")
	}
	duration := stabilityDuration(t, "AWS_COST_EXPORTER_STABILITY_DURATION", 24*time.Hour)
	interval := stabilityDuration(t, "AWS_COST_EXPORTER_STABILITY_INTERVAL", 15*time.Second)
	output := os.Getenv("AWS_COST_EXPORTER_STABILITY_OUTPUT")
	if output == "" {
		output = filepath.Join(t.TempDir(), "stability-result.json")
	}

	const targets = 20
	registry := newV1ReferenceRegistry(t, targets, 1000)
	series := countBusinessSeries(t, registry)
	if series < 20_000 {
		t.Fatalf("stability business series=%d, want at least 20000", series)
	}
	if _, err := registry.Gather(); err != nil {
		t.Fatalf("warm reference registry: %v", err)
	}
	runtime.GC()
	startHeap, startRSS := memoryUsage()
	startGoroutines, startConnections := runtime.NumGoroutine(), openConnections()
	started := time.Now()
	deadline := started.Add(duration)
	latencies := make([]float64, 0, int(duration/interval)+1)
	samples := make([]resourceSample, 0, cap(latencies))
	errors := 0
	for time.Now().Before(deadline) {
		scrapeStarted := time.Now()
		if _, err := registry.Gather(); err != nil {
			errors++
		}
		latencies = append(latencies, float64(time.Since(scrapeStarted))/float64(time.Millisecond))
		heap, rss := memoryUsage()
		samples = append(samples, resourceSample{elapsed: time.Since(started), heap: heap, rss: rss, goroutines: runtime.NumGoroutine(), connections: openConnections()})
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		timer := time.NewTimer(min(interval, remaining))
		<-timer.C
	}
	runtime.GC()
	endHeap, endRSS := memoryUsage()
	result := stabilityResult{
		Version: version.Current().Version, SourceSHA: stabilitySourceSHA(), GoVersion: runtime.Version(),
		Kernel: stabilityKernel(), CPUCount: runtime.NumCPU(), MemoryLimitBytes: stabilityMemoryLimit(),
		DurationSeconds: time.Since(started).Seconds(), Targets: targets, BusinessSeries: series,
		Scrapes: len(latencies), Errors: errors, P99Milliseconds: percentile99(latencies),
		HeapGrowthBytes: endHeap - startHeap, RSSGrowthBytes: endRSS - startRSS,
		GoroutineGrowth: runtime.NumGoroutine() - startGoroutines, ConnectionGrowth: openConnections() - startConnections,
	}
	result.HeapSlopePerHour = resourceSlope(samples, func(sample resourceSample) float64 { return float64(sample.heap) })
	result.RSSSlopePerHour = resourceSlope(samples, func(sample resourceSample) float64 { return float64(sample.rss) })
	result.GoroutineSlope = resourceSlope(samples, func(sample resourceSample) float64 { return float64(sample.goroutines) })
	result.ConnectionSlope = resourceSlope(samples, func(sample resourceSample) float64 { return float64(sample.connections) })
	errorRatio := float64(errors) / float64(max(len(latencies), 1))
	stableSlope := duration < time.Hour || result.HeapSlopePerHour <= 4<<20 && result.RSSSlopePerHour <= 8<<20 &&
		result.GoroutineSlope <= 1 && result.ConnectionSlope <= 1
	result.Passed = result.Scrapes > 0 && errorRatio <= 0.001 && result.P99Milliseconds < 5000 &&
		result.HeapGrowthBytes <= 32<<20 && result.RSSGrowthBytes <= 64<<20 &&
		result.GoroutineGrowth <= 2 && result.ConnectionGrowth <= 2 && stableSlope
	writeStabilityResult(t, output, result)
	if !result.Passed {
		t.Fatalf("v1 stability thresholds failed; report=%s", output)
	}
}

func resourceSlope(samples []resourceSample, value func(resourceSample) float64) float64 {
	if len(samples) < 2 {
		return 0
	}
	samples = samples[len(samples)/2:]
	if len(samples) < 2 {
		return 0
	}
	var sumX, sumY, sumXY, sumXX float64
	for _, sample := range samples {
		x := sample.elapsed.Hours()
		y := value(sample)
		sumX, sumY = sumX+x, sumY+y
		sumXY, sumXX = sumXY+x*y, sumXX+x*x
	}
	n := float64(len(samples))
	denominator := n*sumXX - sumX*sumX
	if denominator == 0 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / denominator
}

func stabilityDuration(t *testing.T, name string, fallback time.Duration) time.Duration {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		t.Fatalf("%s=%q must be a positive duration", name, value)
	}
	return parsed
}

func percentile99(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	ordered := append([]float64(nil), values...)
	slices.Sort(ordered)
	index := (99*len(ordered) + 99) / 100
	return ordered[min(index-1, len(ordered)-1)]
}

func memoryUsage() (heap, rss int64) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	heap = int64(stats.HeapAlloc)
	if runtime.GOOS != "linux" {
		return heap, 0
	}
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return heap, 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return heap, 0
	}
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return heap, 0
	}
	return heap, pages * int64(os.Getpagesize())
}

func openConnections() int {
	if runtime.GOOS != "linux" {
		return 0
	}
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return 0
	}
	connections := 0
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
		if err == nil && strings.HasPrefix(target, "socket:[") {
			connections++
		}
	}
	return connections
}

func stabilitySourceSHA() string {
	if value := os.Getenv("GITHUB_SHA"); value != "" {
		return value
	}
	return version.Current().Revision
}

func stabilityKernel() string {
	if value := os.Getenv("AWS_COST_EXPORTER_STABILITY_KERNEL"); value != "" {
		return value
	}
	return runtime.GOOS + "/" + runtime.GOARCH
}

func stabilityMemoryLimit() int64 {
	value := os.Getenv("AWS_COST_EXPORTER_STABILITY_MEMORY_LIMIT_BYTES")
	limit, _ := strconv.ParseInt(value, 10, 64)
	return limit
}

func writeStabilityResult(t *testing.T, path string, result stabilityResult) {
	t.Helper()
	document, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	document = append(document, '\n')
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatalf("write stability result %s: %v", path, err)
	}
	t.Logf("v1 stability report: %s (p99=%.2fms scrapes=%d errors=%d)", path, result.P99Milliseconds, result.Scrapes, result.Errors)
}
