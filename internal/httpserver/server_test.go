package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

var requiredID = identity.CollectorID{Target: "payer-prod", Name: "total"}

type staticReader struct{ view ports.SnapshotView }

func (value staticReader) Load() ports.SnapshotView { return value.view }

func readyReader() staticReader {
	return staticReader{view: ports.SnapshotView{Collectors: map[identity.CollectorID]ports.CollectorStatus{requiredID: {LastSuccess: time.Now(), Up: true, Freshness: ports.FreshnessFresh}}}}
}

func TestServerRoutesAndMethodPolicy(t *testing.T) {
	registry := prometheus.NewRegistry()
	subject, err := New(testConfig(), registry, readyReader(), []identity.CollectorID{requiredID}, version.Info{Version: "v0.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, path string
		code         int
		body         string
	}{
		{http.MethodGet, "/healthz", 200, `"status":"ok"`}, {http.MethodGet, "/ready", 200, `"status":"ready"`}, {http.MethodGet, "/version", 200, `"version":"v0.2.0"`}, {http.MethodGet, "/metrics", 200, ""}, {http.MethodGet, "/missing", 404, ""}, {http.MethodPost, "/healthz", 405, ""}, {http.MethodHead, "/ready", 200, ""},
	} {
		response := httptest.NewRecorder()
		subject.Handler().ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != test.code || !strings.Contains(response.Body.String(), test.body) {
			t.Fatalf("%s %s = %d %q", test.method, test.path, response.Code, response.Body.String())
		}
		if response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("missing nosniff header")
		}
	}
}

func TestReadinessRequiresEveryRequiredTargetCollector(t *testing.T) {
	for _, test := range []struct {
		name   string
		status ports.CollectorStatus
		code   int
		reason string
	}{
		{"missing", ports.CollectorStatus{}, 503, "missing"}, {"fresh", ports.CollectorStatus{LastSuccess: time.Now(), Freshness: ports.FreshnessFresh}, 200, ""}, {"aging", ports.CollectorStatus{LastSuccess: time.Now(), Freshness: ports.FreshnessAging}, 200, ""}, {"stale", ports.CollectorStatus{LastSuccess: time.Now(), Freshness: ports.FreshnessStale}, 503, "stale"},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := staticReader{view: ports.SnapshotView{Collectors: map[identity.CollectorID]ports.CollectorStatus{requiredID: test.status}}}
			subject, _ := New(testConfig(), prometheus.NewRegistry(), reader, []identity.CollectorID{requiredID}, version.Info{})
			response := httptest.NewRecorder()
			subject.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
			if response.Code != test.code || !strings.Contains(response.Body.String(), test.reason) {
				t.Fatalf("ready=%d %q", response.Code, response.Body.String())
			}
		})
	}
}

func TestServerDebugIsolationAndValidation(t *testing.T) {
	value := testConfig()
	value.Debug.Enabled = true
	subject, err := New(value, prometheus.NewRegistry(), readyReader(), []identity.CollectorID{requiredID}, version.Info{})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	subject.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug", nil))
	if response.Code != 200 {
		t.Fatalf("debug=%d", response.Code)
	}
	for _, mutate := range []func(*config.ServerConfig){func(v *config.ServerConfig) { v.MetricsPath = "/ready" }, func(v *config.ServerConfig) { v.WriteTimeout = 0 }} {
		value := testConfig()
		mutate(&value)
		if server, err := New(value, prometheus.NewRegistry(), readyReader(), []identity.CollectorID{requiredID}, version.Info{}); server != nil || err == nil {
			t.Fatalf("New(invalid)=%#v,%v", server, err)
		}
	}
	if server, err := New(testConfig(), prometheus.NewRegistry(), readyReader(), []identity.CollectorID{requiredID, requiredID}, version.Info{}); server != nil || err == nil {
		t.Fatal("accepted duplicate required ID")
	}
}

func testConfig() config.ServerConfig {
	return config.ServerConfig{ListenAddress: ":0", MetricsPath: "/metrics", ReadHeaderTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: time.Second, ShutdownTimeout: time.Second, MaxInFlight: 2}
}
