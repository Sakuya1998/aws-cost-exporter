package common

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/smithy-go"
)

func TestClassifyErrorCoversSafeKindsAndAccessors(t *testing.T) {
	tests := []struct {
		err   error
		kind  ErrorKind
		retry bool
	}{{context.Canceled, ErrorCanceled, false}, {context.DeadlineExceeded, ErrorTimeout, true}, {timeoutError{}, ErrorTimeout, true}, {&smithy.GenericAPIError{Code: "ThrottlingException", Message: "private"}, ErrorThrottle, true}, {&smithy.GenericAPIError{Code: "AccessDeniedException", Message: "private"}, ErrorAuthorization, false}, {&smithy.GenericAPIError{Code: "ValidationException", Message: "private"}, ErrorValidation, false}, {&smithy.GenericAPIError{Code: "InternalServerException", Message: "private"}, ErrorTransient, true}, {errors.New("private"), ErrorUnknown, false}}
	for _, test := range tests {
		value := ClassifyError(test.err)
		if value.Kind() != test.kind || value.SafeKind() != string(test.kind) || value.Retryable() != test.retry || !errors.Is(value, test.err) || value.Error() != "AWS request failed: "+string(test.kind) {
			t.Fatalf("classified=%#v", value)
		}
	}
	if ClassifyError(nil) != nil {
		t.Fatal("nil classification should stay nil")
	}
}
