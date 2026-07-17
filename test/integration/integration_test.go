package integration_test

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

	"github.com/sakuya1998/aws-cost-exporter/internal/app"
	"github.com/sakuya1998/aws-cost-exporter/internal/config"
)

type costRequest struct {
	TimePeriod struct {
		Start string
		End   string
	}
	NextPageToken string
	GroupBy       []json.RawMessage
}

func TestPaginationPublishesEveryServicePage(t *testing.T) {
	var calls atomic.Int32
	baseURL := runExporter(t, func(writer http.ResponseWriter, request *http.Request) {
		input := decodeRequest(t, request)
		calls.Add(1)
		key, amount, next := "AmazonEC2", "1", `,"NextPageToken":"next"`
		if input.NextPageToken != "" {
			key, amount, next = "AmazonS3", "2", ""
		}
		writeFixture(t, writer, "grouped.json", map[string]string{
			"START": input.TimePeriod.Start, "END": input.TimePeriod.End,
			"KEY": key, "AMOUNT": amount, "NEXT": next,
		})
	}, func(value *config.Config) {
		value.Collection.CostExplorer.Collectors.Service = true
	})
	body := awaitHTTP(t, baseURL+"/metrics", func(code int, body string) bool {
		return code == http.StatusOK &&
			strings.Contains(body, "aws_cost_service_daily_amount{aws_service=\"AmazonEC2\",currency=\"USD\",target=\"integration\"} 1\n") &&
			strings.Contains(body, "aws_cost_service_daily_amount{aws_service=\"AmazonS3\",currency=\"USD\",target=\"integration\"} 2\n")
	})
	if calls.Load() != 4 || !strings.Contains(body, "aws_cost_service_month_to_date_amount") {
		t.Fatalf("pagination calls=%d metrics=%s", calls.Load(), body)
	}
}

func TestThrottleRetriesThenPublishes(t *testing.T) {
	var calls atomic.Int32
	baseURL := runExporter(t, func(writer http.ResponseWriter, request *http.Request) {
		input := decodeRequest(t, request)
		if calls.Add(1) == 1 {
			writer.Header().Set("X-Amzn-Errortype", "ThrottlingException")
			writer.WriteHeader(http.StatusBadRequest)
			writeFixture(t, writer, "error.json", map[string]string{"TYPE": "ThrottlingException"})
			return
		}
		writeFixture(t, writer, "total.json", map[string]string{
			"START": input.TimePeriod.Start, "END": input.TimePeriod.End, "AMOUNT": "7",
		})
	}, func(value *config.Config) {
		value.Collection.CostExplorer.Collectors.Total = true
	})
	body := awaitHTTP(t, baseURL+"/metrics", func(code int, body string) bool {
		return code == http.StatusOK &&
			strings.Contains(body, "aws_cost_daily_amount{currency=\"USD\",target=\"integration\"} 7\n") &&
			strings.Contains(body, "aws_cost_exporter_aws_api_retries_total{operation=\"GetCostAndUsage\",reason=\"throttle\",target=\"integration\"} 1\n")
	})
	if calls.Load() != 3 {
		t.Fatalf("throttle calls=%d metrics=%s", calls.Load(), body)
	}
}

func TestGlobalFiltersInjectedIntoRequests(t *testing.T) {
	var capturedBody atomic.Value
	baseURL := runExporter(t, func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		capturedBody.Store(string(body))
		input := decodeRequestBody(t, body)
		writeFixture(t, writer, "total.json", map[string]string{
			"START": input.TimePeriod.Start, "END": input.TimePeriod.End, "AMOUNT": "3",
		})
	}, func(value *config.Config) {
		value.Collection.CostExplorer.Collectors.Total = true
		value.Targets[0].CostExplorer.Filters.Services = []string{"Amazon EC2"}
		value.Targets[0].CostExplorer.Filters.Regions = []string{"us-east-1"}
	})
	awaitHTTP(t, baseURL+"/metrics", func(code int, body string) bool {
		return code == http.StatusOK && strings.Contains(body, "aws_cost_daily_amount{currency=\"USD\",target=\"integration\"} 3\n")
	})
	got, _ := capturedBody.Load().(string)
	if !strings.Contains(got, "Amazon EC2") || !strings.Contains(got, "us-east-1") {
		t.Fatalf("request missing target filters: %s", got)
	}
}

func TestPartialCollectorFailureKeepsSuccessfulSnapshot(t *testing.T) {
	baseURL := runExporter(t, func(writer http.ResponseWriter, request *http.Request) {
		input := decodeRequest(t, request)
		if len(input.GroupBy) > 0 {
			writer.Header().Set("X-Amzn-Errortype", "AccessDeniedException")
			writer.WriteHeader(http.StatusForbidden)
			writeFixture(t, writer, "error.json", map[string]string{"TYPE": "AccessDeniedException"})
			return
		}
		writeFixture(t, writer, "total.json", map[string]string{
			"START": input.TimePeriod.Start, "END": input.TimePeriod.End, "AMOUNT": "9",
		})
	}, func(value *config.Config) {
		value.Collection.CostExplorer.Collectors.Total = true
		value.Collection.CostExplorer.Collectors.Service = true
	})
	body := awaitHTTP(t, baseURL+"/metrics", func(code int, body string) bool {
		return code == http.StatusOK &&
			strings.Contains(body, "aws_cost_daily_amount{currency=\"USD\",target=\"integration\"} 9\n") &&
			strings.Contains(body, "aws_cost_exporter_collector_up{collector=\"service\",target=\"integration\"} 0\n") &&
			strings.Contains(body, "aws_cost_exporter_collector_up{collector=\"total\",target=\"integration\"} 1\n") &&
			strings.Contains(body, "aws_cost_exporter_refresh_total{collector=\"service\",status=\"error\",target=\"integration\"} 1\n")
	})
	awaitHTTP(t, baseURL+"/ready", func(code int, _ string) bool {
		return code == http.StatusServiceUnavailable
	})
	if strings.Contains(body, "aws_cost_service_daily_amount{") {
		t.Fatalf("failed collector published metrics: %s", body)
	}
}

func TestMaxPagesExceededMarksCollectorDown(t *testing.T) {
	baseURL := runExporter(t, func(writer http.ResponseWriter, request *http.Request) {
		input := decodeRequest(t, request)
		next := `,"NextPageToken":"next"`
		if input.NextPageToken != "" {
			next = ""
		}
		writeFixture(t, writer, "grouped.json", map[string]string{
			"START": input.TimePeriod.Start, "END": input.TimePeriod.End,
			"KEY": "AmazonEC2", "AMOUNT": "1", "NEXT": next,
		})
	}, func(value *config.Config) {
		value.Collection.CostExplorer.Collectors.Service = true
		value.Collection.CostExplorer.MaxPages = 1
	})
	body := awaitHTTP(t, baseURL+"/metrics", func(code int, body string) bool {
		return code == http.StatusOK &&
			strings.Contains(body, "aws_cost_exporter_collector_up{collector=\"service\",target=\"integration\"} 0\n") &&
			strings.Contains(body, "aws_cost_exporter_refresh_total{collector=\"service\",status=\"error\",target=\"integration\"} 1\n")
	})
	if strings.Contains(body, "aws_cost_service_daily_amount{") {
		t.Fatalf("failed pagination published metrics: %s", body)
	}
}

func runExporter(t *testing.T, handler http.HandlerFunc, enable func(*config.Config)) string {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "integration-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "integration-secret-key")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	fakeAWS := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if strings.Contains(string(body), "Action=GetCallerIdentity") {
			writer.Header().Set("Content-Type", "text/xml")
			_, _ = io.WriteString(writer, `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><GetCallerIdentityResult><Account>444455556666</Account><Arn>arn:aws:iam::444455556666:user/integration</Arn><UserId>integration</UserId></GetCallerIdentityResult><ResponseMetadata><RequestId>request-id</RequestId></ResponseMetadata></GetCallerIdentityResponse>`)
			return
		}
		request.Body = io.NopCloser(strings.NewReader(string(body)))
		handler(writer, request)
	}))
	t.Cleanup(fakeAWS.Close)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release address: %v", err)
	}
	value := config.Default()
	value.AWS.Credentials.Sources = map[string]config.CredentialSourceConfig{"runtime": {Type: config.CredentialSourceDefaultChain}}
	value.Targets = []config.TargetConfig{{Name: "integration", AccountID: "444455556666", Required: true, Credentials: config.TargetCredentialsConfig{Source: "runtime"}, CostExplorer: config.TargetCostExplorerConfig{Enabled: true}}}
	value.Server.ListenAddress, value.Server.ShutdownTimeout = address, time.Second
	value.AWS.Endpoints.STS, value.AWS.Endpoints.CostExplorer, value.AWS.RequestTimeout = fakeAWS.URL, fakeAWS.URL, time.Second
	value.AWS.Retry.MaxAttempts, value.AWS.Retry.BaseDelay, value.AWS.Retry.MaxBackoff = 3, time.Millisecond, 5*time.Millisecond
	value.AWS.RateLimit.GlobalRequestsPerSecond, value.AWS.RateLimit.GlobalBurst = 10, 5
	value.AWS.RateLimit.TargetRequestsPerSecond, value.AWS.RateLimit.TargetBurst = 10, 5
	value.Collection.StartupRefresh, value.Collection.JitterRatio = true, 0
	value.Collection.CostExplorer.Collectors = config.CollectorsConfig{}
	value.Telemetry.IncludeGoCollector, value.Telemetry.IncludeProcessCollector = false, false
	enable(&value)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	go func() { result <- app.Run(ctx, value, logger) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-result:
			if err != nil {
				t.Errorf("app.Run shutdown: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("app.Run did not stop")
		}
	})
	baseURL := "http://" + address
	awaitHTTP(t, baseURL+"/healthz", func(code int, _ string) bool { return code == http.StatusOK })
	return baseURL
}

func decodeRequest(t *testing.T, request *http.Request) costRequest {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Errorf("read request body: %v", err)
	}
	return decodeRequestBody(t, body)
}

func decodeRequestBody(t *testing.T, body []byte) costRequest {
	t.Helper()
	var input costRequest
	if err := json.Unmarshal(body, &input); err != nil {
		t.Errorf("decode Cost Explorer request: %v", err)
	}
	return input
}

func writeFixture(t *testing.T, writer http.ResponseWriter, name string, replacements map[string]string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "fixtures", name))
	if err != nil {
		t.Errorf("read fixture %s: %v", name, err)
		return
	}
	body := string(data)
	for key, value := range replacements {
		body = strings.ReplaceAll(body, "{{"+key+"}}", value)
	}
	writer.Header().Set("Content-Type", "application/x-amz-json-1.1")
	_, _ = fmt.Fprint(writer, body)
}

func awaitHTTP(t *testing.T, url string, accept func(int, string) bool) string {
	t.Helper()
	client := &http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		response, err := client.Get(url)
		if err == nil {
			data, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			last = string(data)
			if readErr == nil && accept(response.StatusCode, last) {
				return last
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met for %s; last body=%s", url, last)
	return ""
}
