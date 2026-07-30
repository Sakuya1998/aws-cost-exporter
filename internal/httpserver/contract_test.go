package httpserver

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

var updateHTTPContract = flag.Bool("update-contract", false, "rewrite the reviewed v1 HTTP contract fixture")

type httpContractEntry struct {
	State       string `json:"state"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Status      int    `json:"status"`
	ContentType string `json:"content_type,omitempty"`
	Allow       string `json:"allow,omitempty"`
	Body        string `json:"body,omitempty"`
}

type httpContract struct {
	Version string              `json:"version"`
	Routes  []httpContractEntry `json:"routes"`
}

func TestV1HTTPContract(t *testing.T) {
	ready := contractServer(t, ports.CollectorStatus{LastSuccess: time.Unix(100, 0), Freshness: ports.FreshnessFresh})
	missing := contractServer(t, ports.CollectorStatus{})
	stale := contractServer(t, ports.CollectorStatus{LastSuccess: time.Unix(100, 0), Freshness: ports.FreshnessStale})
	shuttingDown := contractServer(t, ports.CollectorStatus{LastSuccess: time.Unix(100, 0), Freshness: ports.FreshnessFresh})
	shuttingDown.BeginShutdown()

	cases := []struct {
		state, method, path string
		server              *Server
	}{
		{"ready", http.MethodGet, "/metrics", ready}, {"ready", http.MethodHead, "/metrics", ready}, {"ready", http.MethodPost, "/metrics", ready},
		{"ready", http.MethodGet, "/healthz", ready}, {"ready", http.MethodHead, "/healthz", ready}, {"ready", http.MethodPost, "/healthz", ready},
		{"ready", http.MethodGet, "/ready", ready}, {"ready", http.MethodHead, "/ready", ready}, {"ready", http.MethodPost, "/ready", ready},
		{"ready", http.MethodGet, "/version", ready}, {"ready", http.MethodHead, "/version", ready}, {"ready", http.MethodPost, "/version", ready},
		{"ready", http.MethodGet, "/missing", ready},
		{"missing", http.MethodGet, "/ready", missing}, {"stale", http.MethodGet, "/ready", stale}, {"shutting_down", http.MethodGet, "/ready", shuttingDown},
	}
	contract := httpContract{Version: "v1.0.0", Routes: make([]httpContractEntry, 0, len(cases))}
	for _, test := range cases {
		response := httptest.NewRecorder()
		test.server.Handler().ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		contract.Routes = append(contract.Routes, httpContractEntry{
			State: test.state, Method: test.method, Path: test.path, Status: response.Code,
			ContentType: response.Header().Get("Content-Type"), Allow: response.Header().Get("Allow"), Body: response.Body.String(),
		})
	}
	sort.Slice(contract.Routes, func(i, j int) bool {
		left, right := contract.Routes[i], contract.Routes[j]
		if left.State != right.State {
			return left.State < right.State
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		return left.Method < right.Method
	})
	got, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "v1", "http-contract.json")
	if *updateHTTPContract {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read reviewed v1 HTTP fixture: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("v1 HTTP contract changed; review then update fixture\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func contractServer(t *testing.T, status ports.CollectorStatus) *Server {
	t.Helper()
	reader := staticReader{view: ports.SnapshotView{Collectors: map[identity.CollectorID]ports.CollectorStatus{requiredID: status}}}
	server, err := New(testConfig(), prometheus.NewRegistry(), reader, []identity.CollectorID{requiredID}, version.Info{Version: "v1.0.0", Revision: "contract", BuildDate: "2026-07-30T00:00:00Z", GoVersion: "go1.24.0"})
	if err != nil {
		t.Fatal(err)
	}
	return server
}
