// Package main provides the aws-cost-exporter executable entry point.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/sakuya1998/aws-cost-exporter/internal/app"
)

// main maps process signals and command failures to a bounded lifecycle.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "aws-cost-exporter: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output, errorOutput io.Writer) error {
	return app.Execute(ctx, args, output, errorOutput)
}
