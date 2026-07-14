// Package logging constructs structured loggers with mandatory redaction.
package logging

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
)

const redactedValue = "[REDACTED]"

// New constructs an isolated slog logger from application configuration.
func New(value config.LogConfig, output io.Writer) (*slog.Logger, error) {
	if output == nil {
		return nil, errors.New("log output must not be nil")
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.TrimSpace(value.Level))); err != nil {
		return nil, fmt.Errorf("log.level: %w", err)
	}

	options := &slog.HandlerOptions{
		AddSource:   value.AddSource,
		Level:       level,
		ReplaceAttr: redactAttribute,
	}

	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(value.Format)) {
	case "json":
		handler = slog.NewJSONHandler(output, options)
	case "text":
		handler = slog.NewTextHandler(output, options)
	default:
		return nil, fmt.Errorf("log.format: unsupported format %q", value.Format)
	}

	return slog.New(handler), nil
}

// redactAttribute replaces values whose keys indicate credentials or secrets.
func redactAttribute(_ []string, attribute slog.Attr) slog.Attr {
	if isSensitiveKey(attribute.Key) {
		return slog.String(attribute.Key, redactedValue)
	}

	return attribute
}

// isSensitiveKey conservatively identifies secret-bearing structured fields.
func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(key)
	for _, fragment := range []string{
		"access_key",
		"authorization",
		"credential",
		"password",
		"private_key",
		"secret",
		"token",
	} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}

	return false
}
