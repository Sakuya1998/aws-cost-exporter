package costexplorer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/smithy-go"
)

// TestClassifyErrorProducesSafeRetryDecisions verifies AWS messages never
// become labels or public error text.
func TestClassifyErrorProducesSafeRetryDecisions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		err       error
		kind      ErrorKind
		retryable bool
	}{
		{name: "canceled", err: context.Canceled, kind: ErrorCanceled},
		{name: "timeout", err: context.DeadlineExceeded, kind: ErrorTimeout, retryable: true},
		{name: "throttle", err: apiError("LimitExceededException"), kind: ErrorThrottle, retryable: true},
		{name: "too many requests", err: apiError("TooManyRequestsException"), kind: ErrorThrottle, retryable: true},
		{name: "request limit", err: apiError("RequestLimitExceeded"), kind: ErrorThrottle, retryable: true},
		{name: "authorization", err: apiError("AccessDeniedException"), kind: ErrorAuthorization},
		{name: "validation", err: apiError("ValidationException"), kind: ErrorValidation},
		{name: "unavailable", err: apiError("DataUnavailableException"), kind: ErrorTransient, retryable: true},
		{name: "unknown", err: apiError("FutureException"), kind: ErrorUnknown},
	}

	for _, test := range tests {
		classified := ClassifyError(test.err)
		if classified.Kind() != test.kind || classified.Retryable() != test.retryable {
			t.Fatalf("%s: ClassifyError() = (%q, %v), want (%q, %v)", test.name, classified.Kind(), classified.Retryable(), test.kind, test.retryable)
		}
		if classified.SafeKind() != string(test.kind) {
			t.Fatalf("%s: SafeKind() = %q, want %q", test.name, classified.SafeKind(), test.kind)
		}
		if !errors.Is(classified, test.err) || strings.Contains(classified.Error(), "private-request-id") {
			t.Fatalf("%s: classified error lost its cause or leaked AWS text: %v", test.name, classified)
		}
	}
}

// apiError returns a Smithy error carrying intentionally private text.
func apiError(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: "private-request-id"}
}
