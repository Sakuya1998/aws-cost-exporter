package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

// TestRunWritesVersionMetadata verifies the executable delegates to the CLI.
func TestRunWritesVersionMetadata(t *testing.T) {
	t.Parallel()

	var output, errorOutput bytes.Buffer

	if err := run(context.Background(), []string{"--version"}, &output, &errorOutput); err != nil {
		t.Fatalf("run() returned an unexpected error: %v", err)
	}

	want := version.Current().String() + "\n"
	if got := output.String(); got != want {
		t.Fatalf("run() output = %q, want %q", got, want)
	}
}
