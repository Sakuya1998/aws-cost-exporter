package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

// TestServerEndpoints verifies routes, methods, content types, and timeouts.
func TestServerEndpoints(t *testing.T) {
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_metric", Help: "Test metric."}))
	reader := &staticReader{view: ports.SnapshotView{Collectors: map[string]ports.CollectorStatus{
		"total": {LastSuccess: time.Now(), Freshness: ports.FreshnessFresh},
	}}}
	value := testConfig()
	subject, err := New(value, registry, reader, []string{"total"}, version.Info{
		Version: "v0.1.0", Revision: "abc", BuildDate: "2026-07-13", GoVersion: "go1.24.0",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if subject.server.ReadHeaderTimeout != value.ReadHeaderTimeout ||
		subject.server.ReadTimeout != value.ReadTimeout ||
		subject.server.WriteTimeout != value.WriteTimeout ||
		subject.server.IdleTimeout != value.IdleTimeout {
		t.Fatalf("server timeouts = %#v, want configured values", subject.server)
	}
	tests := []struct {
		method, path string
		status       int
		contentType  string
		body         string
	}{
		{http.MethodGet, "/metrics", http.StatusOK, "text/plain", "test_metric 0"},
		{http.MethodHead, "/metrics", http.StatusOK, "text/plain", ""},
		{http.MethodGet, "/healthz", http.StatusOK, "application/json", `{"status":"ok"}`},
		{http.MethodHead, "/healthz", http.StatusOK, "application/json", ""},
		{http.MethodGet, "/ready", http.StatusOK, "application/json", `{"status":"ready"}`},
		{http.MethodGet, "/version", http.StatusOK, "application/json", `"version":"v0.1.0"`},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed, "text/plain", "Method Not Allowed"},
		{http.MethodGet, "/missing", http.StatusNotFound, "text/plain", "404 page not found"},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, nil)
		response := httptest.NewRecorder()
		subject.Handler().ServeHTTP(response, request)
		if response.Code != test.status ||
			!strings.HasPrefix(response.Header().Get("Content-Type"), test.contentType) ||
			!strings.Contains(response.Body.String(), test.body) {
			t.Errorf("%s %s = %d %q %q", test.method, test.path, response.Code, response.Header().Get("Content-Type"), response.Body.String())
		}
		if response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s %s missing nosniff", test.method, test.path)
		}
		if test.method == http.MethodHead && response.Body.Len() != 0 {
			t.Errorf("%s %s returned a response body", test.method, test.path)
		}
		if test.status == http.StatusMethodNotAllowed && response.Header().Get("Allow") != "GET, HEAD" {
			t.Errorf("%s %s Allow = %q", test.method, test.path, response.Header().Get("Allow"))
		}
	}
}

// TestReadinessRequiresNonStaleSuccess verifies bounded readiness reasons.
func TestReadinessRequiresNonStaleSuccess(t *testing.T) {
	tests := []struct {
		name   string
		status ports.CollectorStatus
		code   int
		reason string
	}{
		{name: "missing", code: http.StatusServiceUnavailable, reason: `"reason":"missing"`},
		{name: "unknown freshness", status: ports.CollectorStatus{LastSuccess: time.Now()}, code: http.StatusServiceUnavailable, reason: `"reason":"missing"`},
		{name: "stale", status: ports.CollectorStatus{LastSuccess: time.Now(), Freshness: ports.FreshnessStale}, code: http.StatusServiceUnavailable, reason: `"reason":"stale"`},
		{name: "fresh retained failure", status: ports.CollectorStatus{LastSuccess: time.Now(), Freshness: ports.FreshnessAging}, code: http.StatusOK, reason: `"status":"ready"`},
	}
	for _, test := range tests {
		reader := &staticReader{view: ports.SnapshotView{Collectors: map[string]ports.CollectorStatus{"total": test.status}}}
		subject, _ := New(testConfig(), prometheus.NewRegistry(), reader, []string{"total"}, version.Info{})
		response := httptest.NewRecorder()
		subject.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
		if response.Code != test.code || !strings.Contains(response.Body.String(), test.reason) {
			t.Errorf("%s readiness = %d %q", test.name, response.Code, response.Body.String())
		}
	}
}

// TestDebugRoutesAreConditional verifies debug exposes only fixed diagnostics.
func TestDebugRoutesAreConditional(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		value := testConfig()
		value.Debug.Enabled = enabled
		subject, _ := New(value, prometheus.NewRegistry(), &staticReader{}, []string{"total"}, version.Info{})
		for _, path := range []string{"/debug", "/debug/pprof/goroutine?debug=1", "/debug/pprof/cmdline"} {
			response := httptest.NewRecorder()
			subject.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			want := http.StatusNotFound
			if enabled && path != "/debug/pprof/cmdline" {
				want = http.StatusOK
			}
			if response.Code != want {
				t.Errorf("enabled=%v GET %s = %d, want %d", enabled, path, response.Code, want)
			}
			if path == "/debug" && enabled && response.Body.String() != "{\"status\":\"enabled\"}\n" {
				t.Errorf("debug index exposed unexpected content: %q", response.Body.String())
			}
		}
	}
}

// TestNewRejectsInvalidDependencies verifies fail-fast construction.
func TestNewRejectsInvalidDependencies(t *testing.T) {
	if subject, err := New(config.ServerConfig{}, nil, nil, nil, version.Info{}); subject != nil || err == nil {
		t.Fatalf("New(invalid) = %#v, %v; want error", subject, err)
	}
	var typedNil *staticReader
	if subject, err := New(testConfig(), prometheus.NewRegistry(), typedNil, []string{"total"}, version.Info{}); subject != nil || err == nil {
		t.Fatalf("New(typed nil) = %#v, %v; want error", subject, err)
	}
	reader := &staticReader{}
	for _, value := range []config.ServerConfig{
		func() config.ServerConfig { value := testConfig(); value.MetricsPath = "/ready"; return value }(),
		func() config.ServerConfig { value := testConfig(); value.WriteTimeout = 0; return value }(),
	} {
		if subject, err := New(value, prometheus.NewRegistry(), reader, []string{"total"}, version.Info{}); subject != nil || err == nil {
			t.Fatalf("New(invalid config) = %#v, %v; want error", subject, err)
		}
	}
	if subject, err := New(testConfig(), prometheus.NewRegistry(), reader, []string{"total", "total"}, version.Info{}); subject != nil || err == nil {
		t.Fatalf("New(duplicate required) = %#v, %v; want error", subject, err)
	}
}

type staticReader struct{ view ports.SnapshotView }

func (reader *staticReader) Load() ports.SnapshotView { return reader.view }

func testConfig() config.ServerConfig {
	return config.ServerConfig{
		ListenAddress: ":8080", MetricsPath: "/metrics", MaxInFlight: 2,
		ReadHeaderTimeout: time.Second, ReadTimeout: 2 * time.Second,
		WriteTimeout: 3 * time.Second, IdleTimeout: 4 * time.Second,
		ShutdownTimeout: time.Second,
	}
}
