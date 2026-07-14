package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/logging"
)

// TestNewJSONLoggerRedactsSensitiveAttributes verifies top-level and grouped
// credentials are removed before structured output is written.
func TestNewJSONLoggerRedactsSensitiveAttributes(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger, err := logging.New(config.LogConfig{Level: "info", Format: "json"}, &output)
	if err != nil {
		t.Fatalf("New() returned an unexpected error: %v", err)
	}

	logger.Info(
		"AWS request",
		"aws_secret_access_key", "secret-value",
		slog.Group("auth", "session-token", "token-value"),
		"operation", "GetCostAndUsage",
	)

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode JSON log: %v", err)
	}
	auth, ok := record["auth"].(map[string]any)
	if !ok {
		t.Fatalf("auth attribute = %#v, want object", record["auth"])
	}
	if record["aws_secret_access_key"] != "[REDACTED]" || auth["session-token"] != "[REDACTED]" {
		t.Fatalf("sensitive attributes were not redacted: %#v", record)
	}
	if record["operation"] != "GetCostAndUsage" {
		t.Fatalf("operation = %#v, want preserved value", record["operation"])
	}
}

// TestNewTextLoggerHonorsMinimumLevel verifies format selection and filtering.
func TestNewTextLoggerHonorsMinimumLevel(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger, err := logging.New(config.LogConfig{Level: "warn", Format: "text"}, &output)
	if err != nil {
		t.Fatalf("New() returned an unexpected error: %v", err)
	}

	logger.Info("ignored")
	logger.Warn("visible", "collector", "total")

	got := output.String()
	if strings.Contains(got, "ignored") || !strings.Contains(got, "level=WARN") ||
		!strings.Contains(got, "msg=visible") {
		t.Fatalf("text log output = %q, want one WARN record", got)
	}
}

// TestNewRejectsInvalidConfiguration verifies startup fails before logging with
// an unsupported level or format.
func TestNewRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	tests := []config.LogConfig{
		{Level: "verbose", Format: "json"},
		{Level: "info", Format: "xml"},
	}
	for _, value := range tests {
		if _, err := logging.New(value, &bytes.Buffer{}); err == nil {
			t.Errorf("New(%+v) returned nil error", value)
		}
	}
}
