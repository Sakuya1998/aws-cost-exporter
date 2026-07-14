// Package httpserver exposes metrics and bounded operational endpoints.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/pprof"
	"reflect"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

// net/http/pprof registers DefaultServeMux as an import side effect; Server
// intentionally serves only its private handler and must never serve that mux.

// ReadinessReader supplies one atomic collector status view.
type ReadinessReader interface {
	Load() ports.SnapshotView
}

// ErrInvalidConfig indicates invalid server dependencies or routing.
var ErrInvalidConfig = errors.New("invalid HTTP server configuration")

// Server owns an isolated HTTP handler and its timeout configuration.
type Server struct {
	server          *http.Server
	shutdownTimeout time.Duration
}

// New validates dependencies and constructs an isolated HTTP server.
func New(value config.ServerConfig, gatherer prometheus.Gatherer, reader ReadinessReader, required []string, build version.Info) (*Server, error) {
	metricsTimeout := value.WriteTimeout / 2
	if isNil(gatherer) || isNil(reader) || value.ListenAddress == "" ||
		!strings.HasPrefix(value.MetricsPath, "/") || value.MetricsPath == "/" ||
		value.MetricsPath == "/healthz" || value.MetricsPath == "/ready" ||
		value.MetricsPath == "/version" || value.MaxInFlight <= 0 ||
		value.ReadHeaderTimeout <= 0 || value.ReadTimeout <= 0 ||
		metricsTimeout <= 0 || value.IdleTimeout <= 0 ||
		value.ShutdownTimeout <= 0 || len(required) == 0 {
		return nil, ErrInvalidConfig
	}
	names := make([]string, 0, len(required))
	known := make(map[string]struct{}, len(required))
	for _, name := range required {
		name = strings.TrimSpace(name)
		if _, duplicate := known[name]; name == "" || duplicate {
			return nil, ErrInvalidConfig
		}
		known[name] = struct{}{}
		names = append(names, name)
	}
	metrics := getOnly(promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{
		Timeout: metricsTimeout, MaxRequestsInFlight: value.MaxInFlight,
		EnableOpenMetrics: true,
	}))
	health := getOnly(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		writeJSON(response, request, http.StatusOK, statusResponse{Status: "ok"})
	}))
	ready := getOnly(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		code, payload := readiness(reader.Load(), names)
		writeJSON(response, request, code, payload)
	}))
	versionHandler := getOnly(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		writeJSON(response, request, http.StatusOK, build)
	}))
	debugIndex := getOnly(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		writeJSON(response, request, http.StatusOK, statusResponse{Status: "enabled"})
	}))
	debugPprof := getOnly(http.HandlerFunc(servePprof))
	root := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if value.Debug.Enabled {
			switch {
			case request.URL.Path == "/debug":
				debugIndex.ServeHTTP(response, request)
				return
			case request.URL.Path == "/debug/pprof" || strings.HasPrefix(request.URL.Path, "/debug/pprof/"):
				debugPprof.ServeHTTP(response, request)
				return
			}
		}
		switch request.URL.Path {
		case value.MetricsPath:
			metrics.ServeHTTP(response, request)
		case "/healthz":
			health.ServeHTTP(response, request)
		case "/ready":
			ready.ServeHTTP(response, request)
		case "/version":
			versionHandler.ServeHTTP(response, request)
		default:
			http.NotFound(response, request)
		}
	})
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("X-Content-Type-Options", "nosniff")
		root.ServeHTTP(response, request)
	})
	return &Server{
		server: &http.Server{
			Addr: value.ListenAddress, Handler: handler,
			ReadHeaderTimeout: value.ReadHeaderTimeout, ReadTimeout: value.ReadTimeout,
			WriteTimeout: value.WriteTimeout, IdleTimeout: value.IdleTimeout,
		},
		shutdownTimeout: value.ShutdownTimeout,
	}, nil
}

// Handler returns the isolated application handler for serving or testing.
func (server *Server) Handler() http.Handler { return server.server.Handler }

// ListenAndServe starts serving with the configured network timeouts.
func (server *Server) ListenAndServe() error { return server.server.ListenAndServe() }

// Serve runs on an existing listener with the configured network timeouts.
func (server *Server) Serve(listener net.Listener) error { return server.server.Serve(listener) }

// Shutdown stops new connections and drains active requests to the deadline.
func (server *Server) Shutdown(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, server.shutdownTimeout)
	defer cancel()
	return server.server.Shutdown(ctx)
}

// ShutdownTimeout returns the configured graceful shutdown deadline.
func (server *Server) ShutdownTimeout() time.Duration { return server.shutdownTimeout }

type statusResponse struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func readiness(view ports.SnapshotView, required []string) (int, statusResponse) {
	for _, name := range required {
		status, exists := view.Collectors[name]
		if !exists || status.LastSuccess.IsZero() {
			return http.StatusServiceUnavailable, statusResponse{Status: "not_ready", Reason: "missing"}
		}
		switch status.Freshness {
		case ports.FreshnessFresh, ports.FreshnessAging:
		case ports.FreshnessStale:
			return http.StatusServiceUnavailable, statusResponse{Status: "not_ready", Reason: "stale"}
		default:
			return http.StatusServiceUnavailable, statusResponse{Status: "not_ready", Reason: "missing"}
		}
	}
	return http.StatusOK, statusResponse{Status: "ready"}
}

func getOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			response.Header().Set("Allow", "GET, HEAD")
			http.Error(response, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		if request.Method == http.MethodHead {
			next.ServeHTTP(headWriter{ResponseWriter: response}, request)
			return
		}
		next.ServeHTTP(response, request)
	})
}

func servePprof(response http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/debug/pprof/cmdline":
		http.NotFound(response, request)
	case "/debug/pprof/profile":
		pprof.Profile(response, request)
	case "/debug/pprof/symbol":
		pprof.Symbol(response, request)
	case "/debug/pprof/trace":
		pprof.Trace(response, request)
	default:
		pprof.Index(response, request)
	}
}

type headWriter struct{ http.ResponseWriter }

func (writer headWriter) Write(body []byte) (int, error) { return len(body), nil }

func writeJSON(response http.ResponseWriter, request *http.Request, code int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(code)
	if request.Method != http.MethodHead {
		_ = json.NewEncoder(response).Encode(value)
	}
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
