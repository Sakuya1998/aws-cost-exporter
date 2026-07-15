package app_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sakuya1998/aws-cost-exporter/internal/app"
	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/httpserver"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

type blockingScheduler struct{}

func (blockingScheduler) Run(ctx context.Context) {
	<-ctx.Done()
	time.Sleep(2 * time.Second)
}

type staticReader struct{}

func (staticReader) Load() ports.SnapshotView {
	return ports.SnapshotView{Collectors: map[string]ports.CollectorStatus{
		"total": {LastSuccess: time.Unix(1, 0).UTC(), LastAttempt: time.Unix(1, 0).UTC(), Up: true},
	}}
}

func TestRunServicesTimesOutWaitingForScheduler(t *testing.T) {
	value := config.Default()
	value.Server.ShutdownTimeout = 50 * time.Millisecond
	server, err := httpserver.New(value.Server, prometheus.NewRegistry(), staticReader{}, []string{"total"}, version.Info{})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	go func() {
		done <- app.RunServices(ctx, blockingScheduler{}, server, listener, value.Server.ShutdownTimeout, logger)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunServices() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunServices did not return before test timeout")
	}
}

func TestRunServicesTimesOutWhenServerExitsFirst(t *testing.T) {
	value := config.Default()
	value.Server.ShutdownTimeout = 50 * time.Millisecond
	server, err := httpserver.New(value.Server, prometheus.NewRegistry(), staticReader{}, []string{"total"}, version.Info{})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	done := make(chan error, 1)
	go func() {
		done <- app.RunServices(context.Background(), blockingScheduler{}, server, listener, value.Server.ShutdownTimeout, logger)
	}()
	time.Sleep(20 * time.Millisecond)
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	select {
	case err := <-done:
		if err != nil && !strings.Contains(err.Error(), "closed network connection") {
			t.Fatalf("RunServices() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunServices did not return before test timeout")
	}
}
