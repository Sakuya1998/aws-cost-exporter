package version_test

import (
	"runtime"
	"testing"

	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

// TestCurrentReturnsDevelopmentMetadata verifies source builds carry safe,
// explicit development values before release metadata is injected.
func TestCurrentReturnsDevelopmentMetadata(t *testing.T) {
	t.Parallel()

	got := version.Current()

	if got.Version != "dev" {
		t.Errorf("Version = %q, want %q", got.Version, "dev")
	}
	if got.Revision != "unknown" {
		t.Errorf("Revision = %q, want %q", got.Revision, "unknown")
	}
	if got.BuildDate != "unknown" {
		t.Errorf("BuildDate = %q, want %q", got.BuildDate, "unknown")
	}
	if got.GoVersion != runtime.Version() {
		t.Errorf("GoVersion = %q, want %q", got.GoVersion, runtime.Version())
	}
}

// TestInfoString verifies the command-line representation remains stable.
func TestInfoString(t *testing.T) {
	t.Parallel()

	info := version.Info{
		Version:   "v1.2.3",
		Revision:  "abc123",
		BuildDate: "2026-07-10T09:00:00Z",
		GoVersion: "go1.24.0",
	}

	const want = "aws-cost-exporter version=v1.2.3 revision=abc123 build_date=2026-07-10T09:00:00Z go_version=go1.24.0"
	if got := info.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
