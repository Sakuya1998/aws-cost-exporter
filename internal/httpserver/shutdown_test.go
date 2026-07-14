package httpserver

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

// TestShutdownStopsNewRequestsAndWaitsForInFlight verifies graceful draining.
func TestShutdownStopsNewRequestsAndWaitsForInFlight(t *testing.T) {
	gatherer := &blockingGatherer{started: make(chan struct{}), release: make(chan struct{})}
	subject, err := New(testConfig(), gatherer, &staticReader{}, []string{"total"}, version.Info{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	served := make(chan error, 1)
	go func() { served <- subject.Serve(listener) }()
	client := &http.Client{Timeout: 2 * time.Second}
	url := "http://" + listener.Addr().String()
	requested := make(chan error, 1)
	go func() {
		response, requestErr := client.Get(url + "/metrics")
		if requestErr == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			requestErr = response.Body.Close()
		}
		requested <- requestErr
	}()
	waitFor(t, gatherer.started)
	shutdown := make(chan error, 1)
	go func() { shutdown <- subject.Shutdown(context.Background()) }()
	waitUntilRejected(t, client, url+"/healthz")
	select {
	case err := <-shutdown:
		t.Fatalf("Shutdown() returned before in-flight request completed: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(gatherer.release)
	if err := waitFor(t, requested); err != nil {
		t.Fatalf("in-flight request failed: %v", err)
	}
	if err := waitFor(t, shutdown); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := waitFor(t, served); !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Serve() error = %v, want ErrServerClosed", err)
	}
}

// TestShutdownHonorsConfiguredDeadline verifies bounded draining.
func TestShutdownHonorsConfiguredDeadline(t *testing.T) {
	gatherer := &blockingGatherer{started: make(chan struct{}), release: make(chan struct{})}
	value := testConfig()
	value.ShutdownTimeout = 20 * time.Millisecond
	if value.WriteTimeout/2 <= value.ShutdownTimeout {
		t.Fatal("metrics timeout must exceed shutdown timeout for this test")
	}
	subject, _ := New(value, gatherer, &staticReader{}, []string{"total"}, version.Info{})
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	served := make(chan error, 1)
	go func() { served <- subject.Serve(listener) }()
	requested := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + listener.Addr().String() + "/metrics")
		if err == nil {
			_ = response.Body.Close()
		}
		requested <- err
	}()
	waitFor(t, gatherer.started)
	if err := subject.Shutdown(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want deadline exceeded", err)
	}
	close(gatherer.release)
	if err := waitFor(t, requested); err != nil {
		t.Fatalf("in-flight request failed after deadline: %v", err)
	}
	if err := waitFor(t, served); !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Serve() error = %v, want ErrServerClosed", err)
	}
}

type blockingGatherer struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (gatherer *blockingGatherer) Gather() ([]*dto.MetricFamily, error) {
	gatherer.once.Do(func() { close(gatherer.started) })
	<-gatherer.release
	return nil, nil
}

func waitFor[Value any](t *testing.T, channel <-chan Value) Value {
	t.Helper()
	select {
	case value := <-channel:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for HTTP server event")
		var zero Value
		return zero
	}
}

func waitUntilRejected(t *testing.T, client *http.Client, url string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		response, err := client.Get(url)
		if err != nil {
			return
		}
		_ = response.Body.Close()
		if time.Now().After(deadline) {
			t.Fatal("server continued accepting requests during shutdown")
		}
		time.Sleep(time.Millisecond)
	}
}
