package costexplorer

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/aws/smithy-go"
)

// ErrorKind is a bounded Cost Explorer failure category.
type ErrorKind string

const (
	// ErrorCanceled indicates caller cancellation.
	ErrorCanceled ErrorKind = "canceled"
	// ErrorTimeout indicates a retryable deadline or network timeout.
	ErrorTimeout ErrorKind = "timeout"
	// ErrorThrottle indicates AWS request throttling.
	ErrorThrottle ErrorKind = "throttle"
	// ErrorAuthorization indicates invalid or insufficient credentials.
	ErrorAuthorization ErrorKind = "authorization"
	// ErrorValidation indicates a permanent request error.
	ErrorValidation ErrorKind = "validation"
	// ErrorTransient indicates a retryable service or network failure.
	ErrorTransient ErrorKind = "transient"
	// ErrorUnknown indicates an unrecognized permanent failure.
	ErrorUnknown ErrorKind = "unknown"
)

// ClassifiedError preserves a cause while exposing only safe metadata.
type ClassifiedError struct {
	cause     error
	kind      ErrorKind
	retryable bool
}

// Error returns bounded text without AWS messages or request identifiers.
func (classified *ClassifiedError) Error() string {
	return "Cost Explorer request failed: " + string(classified.kind)
}

// Unwrap returns the original failure for programmatic inspection.
func (classified *ClassifiedError) Unwrap() error { return classified.cause }

// Kind returns the bounded failure category.
func (classified *ClassifiedError) Kind() ErrorKind { return classified.kind }

// SafeKind returns the bounded failure category for infrastructure logging.
func (classified *ClassifiedError) SafeKind() string { return string(classified.kind) }

// Retryable reports whether refresh-level retry is safe.
func (classified *ClassifiedError) Retryable() bool { return classified.retryable }

// ClassifyError converts arbitrary failures into safe operational metadata.
func ClassifyError(err error) *ClassifiedError {
	if err == nil {
		return nil
	}
	classified := &ClassifiedError{cause: err, kind: ErrorUnknown}
	if errors.Is(err, context.Canceled) {
		classified.kind = ErrorCanceled
		return classified
	}
	if errors.Is(err, context.DeadlineExceeded) {
		classified.kind, classified.retryable = ErrorTimeout, true
		return classified
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		classified.kind, classified.retryable = ErrorTransient, true
		if networkError.Timeout() {
			classified.kind = ErrorTimeout
		}
		return classified
	}
	var apiError smithy.APIError
	if !errors.As(err, &apiError) {
		return classified
	}
	switch code := strings.ToLower(apiError.ErrorCode()); {
	case isThrottleCode(code):
		classified.kind, classified.retryable = ErrorThrottle, true
	case strings.Contains(code, "accessdenied") || strings.Contains(code, "unauthorized"):
		classified.kind = ErrorAuthorization
	case strings.Contains(code, "validation") || strings.Contains(code, "invalid"):
		classified.kind = ErrorValidation
	case strings.Contains(code, "unavailable") || strings.Contains(code, "internal"):
		classified.kind, classified.retryable = ErrorTransient, true
	}
	return classified
}
