package common

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/smithy-go"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
)

type countingLimiter struct {
	calls int
	err   error
}

func (value *countingLimiter) Wait(context.Context) error { value.calls++; return value.err }
func TestDualLimiterOrdersEveryAttemptAndCancellation(t *testing.T) {
	global, target := &countingLimiter{}, &countingLimiter{}
	limiter := DualLimiter{Global: global, Target: target}
	for range 3 {
		if err := limiter.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if global.calls != 3 || target.calls != 3 {
		t.Fatalf("global=%d target=%d", global.calls, target.calls)
	}
	global.err = context.Canceled
	if err := limiter.Wait(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait()=%v", err)
	}
	if target.calls != 3 {
		t.Fatal("target limiter ran after global cancellation")
	}
}

func TestRetryObserverPreservesUnderlyingAttemptToken(t *testing.T) {
	limiter := &countingLimiter{}
	observer := retryObserver{Retryer: aws.NopRetryer{}, target: "a", operation: OperationGetCostAndUsage, limiter: limiter, observer: DiscardObserver{}}
	if _, err := observer.GetAttemptToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if limiter.calls != 1 {
		t.Fatalf("limiter calls=%d", limiter.calls)
	}
}

type recordingObserver struct {
	requests, retries         int
	target                    identity.TargetID
	operation, status, reason string
}

func (value *recordingObserver) ObserveRequest(target identity.TargetID, operation, status string, _ time.Duration) {
	value.requests++
	value.target, value.operation, value.status = target, operation, status
}
func (value *recordingObserver) ObserveRetry(target identity.TargetID, operation, reason string) {
	value.retries++
	value.target, value.operation, value.reason = target, operation, reason
}
func (*recordingObserver) ObservePaginationPage(identity.TargetID, string) {}

func TestWrappedRetryerObservesAuthorizedRetriesAndEveryAttempt(t *testing.T) {
	global, target := &countingLimiter{}, &countingLimiter{}
	events := &recordingObserver{}
	wrapped := WrapRetryer(aws.NopRetryer{}, "payer", OperationGetCostAndUsage, DualLimiter{Global: global, Target: target}, events)
	v2, ok := wrapped.(aws.RetryerV2)
	if !ok {
		t.Fatal("wrapped retryer lost RetryerV2")
	}
	for range 3 {
		release, err := v2.GetAttemptToken(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if release != nil {
			_ = release(nil)
		}
	}
	if _, err := wrapped.GetRetryToken(context.Background(), &smithy.GenericAPIError{Code: "ThrottlingException", Message: "private"}); err != nil {
		t.Fatal(err)
	}
	if global.calls != 3 || target.calls != 3 || events.retries != 1 || events.reason != "throttle" {
		t.Fatalf("global=%d target=%d events=%#v", global.calls, target.calls, events)
	}
}

type failingAttemptRetryer struct{ aws.NopRetryer }

func (failingAttemptRetryer) GetAttemptToken(context.Context) (func(error) error, error) {
	return nil, errors.New("underlying token failed")
}
func TestAttemptLimiterAndUnderlyingFailuresArePreserved(t *testing.T) {
	limiter := &countingLimiter{err: context.Canceled}
	observer := retryObserver{Retryer: failingAttemptRetryer{}, limiter: limiter, observer: DiscardObserver{}}
	if _, err := observer.GetAttemptToken(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("limiter=%v", err)
	}
	limiter.err = nil
	if _, err := observer.GetAttemptToken(context.Background()); err == nil || err.Error() != "underlying token failed" {
		t.Fatalf("underlying=%v", err)
	}
}

func TestStatusObservationLimiterAndRetryConstruction(t *testing.T) {
	events := &recordingObserver{}
	ObserveCall(events, "payer", OperationDescribeBudgets, time.Now(), nil)
	if events.requests != 1 || events.status != "success" {
		t.Fatalf("events=%#v", events)
	}
	if RequestStatus(context.Canceled) != "canceled" || RequestStatus(&smithy.GenericAPIError{Code: "Throttling", Message: "private"}) != "throttle" || RequestStatus(errors.New("x")) != "error" {
		t.Fatal("request status bounds failed")
	}
	if RetryReason(&timeoutError{}) != "timeout" || RetryReason(errors.New("x")) != "other" {
		t.Fatal("retry reason bounds failed")
	}
	if !IsThrottleCode("LimitExceededException") || IsThrottleCode("private") {
		t.Fatal("throttle enum failed")
	}
	if err := NewLimiter(1000, 1).Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	retryer := NewRetryer(config.RetryConfig{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxBackoff: time.Second})
	if retryer == nil {
		t.Fatal("nil retryer")
	}
	if delay, err := (jitterBackoff{base: time.Second, max: 2 * time.Second}).BackoffDelay(4, nil); err != nil || delay < 0 || delay > 2*time.Second {
		t.Fatalf("delay=%v err=%v", delay, err)
	}
	DiscardObserver{}.ObserveRequest("a", "op", "status", 0)
	DiscardObserver{}.ObserveRetry("a", "op", "reason")
	DiscardObserver{}.ObservePaginationPage("a", "op")
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }
