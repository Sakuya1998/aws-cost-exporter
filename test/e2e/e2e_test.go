package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

func TestBinaryReadinessMetricsDebugAndTermination(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "e2e-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "e2e-secret-key")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	release := make(chan struct{})
	var releaseOnce sync.Once
	var awsCalls atomic.Int32
	fakeAWS := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		awsCalls.Add(1)
		if !strings.HasSuffix(request.Header.Get("X-Amz-Target"), ".GetCostAndUsage") {
			t.Errorf("unexpected AWS target %q", request.Header.Get("X-Amz-Target"))
		}
		var input struct {
			TimePeriod struct{ Start, End string }
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Errorf("decode request: %v", err)
		}
		<-release
		template, err := os.ReadFile(filepath.Join("..", "fixtures", "total.json"))
		if err != nil {
			t.Errorf("read response fixture: %v", err)
			return
		}
		body := strings.NewReplacer(
			"{{START}}", input.TimePeriod.Start, "{{END}}", input.TimePeriod.End, "{{AMOUNT}}", "11",
		).Replace(string(template))
		writer.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = fmt.Fprint(writer, body)
	}))
	t.Cleanup(fakeAWS.Close)
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	address := freeAddress(t)
	configPath := writeConfig(t, address, fakeAWS.URL)
	binary := buildBinary(t)
	logFile, err := os.Create(filepath.Join(t.TempDir(), "exporter.log"))
	if err != nil {
		t.Fatalf("create process log: %v", err)
	}
	t.Cleanup(func() { _ = logFile.Close() })
	t.Cleanup(func() {
		if t.Failed() {
			_ = logFile.Sync()
			if logs, readErr := os.ReadFile(logFile.Name()); readErr == nil {
				t.Logf("AWS calls=%d\nbinary logs:\n%s", awsCalls.Load(), logs)
			}
		}
	})
	command := exec.Command(binary, "--config", configPath)
	command.Env, command.Stdout, command.Stderr = cleanExporterEnv(), logFile, logFile
	if err := command.Start(); err != nil {
		t.Fatalf("start binary: %v", err)
	}
	result, stopped := make(chan error, 1), false
	go func() { result <- command.Wait() }()
	t.Cleanup(func() {
		if !stopped {
			_, _ = terminateProcess(command.Process)
			<-result
		}
	})

	baseURL := "http://" + address
	awaitStatus(t, baseURL+"/ready", http.StatusServiceUnavailable)
	awaitStatus(t, baseURL+"/debug", http.StatusNotFound)
	releaseOnce.Do(func() { close(release) })
	awaitStatus(t, baseURL+"/ready", http.StatusOK)
	metrics := fetch(t, baseURL+"/metrics")
	assertGoldenMetrics(t, metrics)

	graceful, err := terminateProcess(command.Process)
	if err != nil {
		t.Fatalf("terminate binary: %v", err)
	}
	select {
	case err := <-result:
		stopped = true
		if graceful && err != nil {
			t.Fatalf("SIGTERM exit: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("binary did not exit")
	}
	awaitUnavailable(t, baseURL+"/healthz")
}

func TestBinaryPublishesServiceAndTotalMetrics(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "e2e-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "e2e-secret-key")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	release := make(chan struct{})
	var releaseOnce sync.Once
	fakeAWS := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasSuffix(request.Header.Get("X-Amz-Target"), ".GetCostAndUsage") {
			t.Errorf("unexpected AWS target %q", request.Header.Get("X-Amz-Target"))
		}
		var input struct {
			TimePeriod struct{ Start, End string }
			GroupBy    []struct{ Key string }
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Errorf("decode request: %v", err)
		}
		<-release
		fixture := "total.json"
		replacements := map[string]string{
			"START": input.TimePeriod.Start, "END": input.TimePeriod.End,
		}
		if len(input.GroupBy) > 0 {
			fixture = "grouped.json"
			replacements["KEY"] = "Amazon EC2"
			replacements["AMOUNT"] = "5"
			replacements["NEXT"] = ""
		} else {
			replacements["AMOUNT"] = "11"
		}
		template, err := os.ReadFile(filepath.Join("..", "fixtures", fixture))
		if err != nil {
			t.Errorf("read response fixture: %v", err)
			return
		}
		body := string(template)
		for key, value := range replacements {
			body = strings.ReplaceAll(body, "{{"+key+"}}", value)
		}
		writer.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = fmt.Fprint(writer, body)
	}))
	t.Cleanup(fakeAWS.Close)
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	address := freeAddress(t)
	configPath := writeMultiCollectorConfig(t, address, fakeAWS.URL)
	binary := buildBinary(t)
	logFile, err := os.Create(filepath.Join(t.TempDir(), "exporter.log"))
	if err != nil {
		t.Fatalf("create process log: %v", err)
	}
	t.Cleanup(func() { _ = logFile.Close() })
	command := exec.Command(binary, "--config", configPath)
	command.Env, command.Stdout, command.Stderr = cleanExporterEnv(), logFile, logFile
	if err := command.Start(); err != nil {
		t.Fatalf("start binary: %v", err)
	}
	result, stopped := make(chan error, 1), false
	go func() { result <- command.Wait() }()
	t.Cleanup(func() {
		if !stopped {
			_, _ = terminateProcess(command.Process)
			<-result
		}
	})

	baseURL := "http://" + address
	releaseOnce.Do(func() { close(release) })
	awaitStatus(t, baseURL+"/ready", http.StatusOK)
	metrics := fetch(t, baseURL+"/metrics")
	for _, fragment := range []string{
		"aws_cost_daily_amount{currency=\"USD\"} 11",
		"aws_cost_service_daily_amount{aws_service=\"Amazon EC2\",currency=\"USD\"} 5",
	} {
		if !strings.Contains(metrics, fragment) {
			t.Fatalf("metrics missing %q\n%s", fragment, metrics)
		}
	}

	graceful, err := terminateProcess(command.Process)
	if err != nil {
		t.Fatalf("terminate binary: %v", err)
	}
	select {
	case err := <-result:
		stopped = true
		if graceful && err != nil {
			t.Fatalf("SIGTERM exit: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("binary did not exit")
	}
}

func cleanExporterEnv() []string {
	result := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(value), "AWS_COST_EXPORTER_") {
			result = append(result, value)
		}
	}
	return result
}

func buildBinary(t *testing.T) string {
	t.Helper()
	name := "aws-cost-exporter"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	command := exec.Command("go", "build", "-o", binary, "./cmd/aws-cost-exporter")
	command.Dir = filepath.Join("..", "..")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build binary: %v\n%s", err, output)
	}
	return binary
}

func writeConfig(t *testing.T, address, endpoint string) string {
	t.Helper()
	content := fmt.Sprintf(`server:
  listen_address: %q
  shutdown_timeout: 1s
aws:
  endpoint_url: %q
  request_timeout: 5s
  retry:
    max_attempts: 1
    base_delay: 1ms
    max_backoff: 5ms
  rate_limit:
    requests_per_second: 1
    burst: 10
cost_explorer:
  startup_refresh: true
  jitter_ratio: 0
  forecast:
    enabled: false
  collectors:
    total: true
    service: false
    region: false
    account: false
telemetry:
  include_go_collector: false
  include_process_collector: false
scheduler:
  failure_backoff:
    initial: 10ms
    max: 50ms
    multiplier: 2
`, address, endpoint)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func writeMultiCollectorConfig(t *testing.T, address, endpoint string) string {
	t.Helper()
	content := fmt.Sprintf(`server:
  listen_address: %q
  shutdown_timeout: 1s
aws:
  endpoint_url: %q
  request_timeout: 5s
  retry:
    max_attempts: 1
    base_delay: 1ms
    max_backoff: 5ms
  rate_limit:
    requests_per_second: 1
    burst: 10
cost_explorer:
  startup_refresh: true
  jitter_ratio: 0
  forecast:
    enabled: false
  collectors:
    total: true
    service: true
    region: false
    account: false
telemetry:
  include_go_collector: false
  include_process_collector: false
scheduler:
  failure_backoff:
    initial: 10ms
    max: 50ms
    multiplier: 2
`, address, endpoint)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func assertGoldenMetrics(t *testing.T, body string) {
	t.Helper()
	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse Prometheus metrics: %v", err)
	}
	var actual bytes.Buffer
	for _, name := range []string{"aws_cost_daily_amount", "aws_cost_month_to_date_amount"} {
		family, exists := families[name]
		if !exists {
			t.Fatalf("metric family %s is missing", name)
		}
		if _, err := expfmt.MetricFamilyToText(&actual, family); err != nil {
			t.Fatalf("encode metric family %s: %v", name, err)
		}
	}
	expected, err := os.ReadFile(filepath.Join("..", "fixtures", "metrics.golden"))
	if err != nil {
		t.Fatalf("read metrics golden: %v", err)
	}
	expected = bytes.ReplaceAll(expected, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(actual.Bytes(), expected) {
		t.Fatalf("metrics differ\nwant:\n%s\ngot:\n%s", expected, actual.Bytes())
	}
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release address: %v", err)
	}
	return address
}

func awaitStatus(t *testing.T, url string, want int) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	var lastCode int
	var lastBody string
	for time.Now().Before(deadline) {
		response, err := (&http.Client{Timeout: 200 * time.Millisecond}).Get(url)
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			_ = response.Body.Close()
			lastCode, lastBody = response.StatusCode, string(body)
			if response.StatusCode == want {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s did not return %d; last status=%d body=%s", url, want, lastCode, lastBody)
}

func fetch(t *testing.T, url string) string {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status=%d error=%v", url, response.StatusCode, err)
	}
	return string(body)
}

func awaitUnavailable(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		response, err := (&http.Client{Timeout: 100 * time.Millisecond}).Get(url)
		if err != nil {
			return
		}
		_ = response.Body.Close()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s remained available after termination", url)
}
