package common

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/aws/smithy-go"
)

// ErrorKind is a bounded AWS failure category.
type ErrorKind string

const (
	ErrorCanceled      ErrorKind = "canceled"
	ErrorTimeout       ErrorKind = "timeout"
	ErrorThrottle      ErrorKind = "throttle"
	ErrorAuthorization ErrorKind = "authorization"
	ErrorValidation    ErrorKind = "validation"
	ErrorTransient     ErrorKind = "transient"
	ErrorUnknown       ErrorKind = "unknown"
)

// ClassifiedError preserves a cause while exposing only safe metadata.
type ClassifiedError struct {
	cause     error
	kind      ErrorKind
	retryable bool
}

func (value *ClassifiedError) Error() string    { return "AWS request failed: " + string(value.kind) }
func (value *ClassifiedError) Unwrap() error    { return value.cause }
func (value *ClassifiedError) Kind() ErrorKind  { return value.kind }
func (value *ClassifiedError) SafeKind() string { return string(value.kind) }
func (value *ClassifiedError) Retryable() bool  { return value.retryable }

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
	code := strings.ToLower(apiError.ErrorCode())
	switch {
	case IsThrottleCode(apiError.ErrorCode()):
		classified.kind, classified.retryable = ErrorThrottle, true
	case strings.Contains(code, "accessdenied") || strings.Contains(code, "unauthorized") || strings.Contains(code, "notauthorized"):
		classified.kind = ErrorAuthorization
	case strings.Contains(code, "validation") || strings.Contains(code, "invalid"):
		classified.kind = ErrorValidation
	case strings.Contains(code, "unavailable") || strings.Contains(code, "internal") || strings.Contains(code, "serviceexception"):
		classified.kind, classified.retryable = ErrorTransient, true
	}
	return classified
}
