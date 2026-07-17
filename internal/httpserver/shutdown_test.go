package httpserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

func TestServerServeAndShutdown(t *testing.T) {
	server, err := New(testConfig(), prometheus.NewRegistry(), readyReader(), []identity.CollectorID{requiredID}, version.Info{})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	client := http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if err := server.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if serveErr := <-done; !errors.Is(serveErr, http.ErrServerClosed) {
		t.Fatalf("Serve()=%v", serveErr)
	}
}

func TestShutdownHonorsConfiguredTimeout(t *testing.T) {
	value := testConfig()
	value.ShutdownTimeout = 20 * time.Millisecond
	server, _ := New(value, prometheus.NewRegistry(), readyReader(), []identity.CollectorID{requiredID}, version.Info{})
	requestStarted := make(chan struct{})
	release := make(chan struct{})
	server.server.Handler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) { close(requestStarted); <-release })
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	go func() { _, _ = http.Get("http://" + listener.Addr().String() + "/healthz") }()
	<-requestStarted
	err := server.Shutdown(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown()=%v", err)
	}
	close(release)
	_ = listener.Close()
	<-done
}
