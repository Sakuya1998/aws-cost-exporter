package costexplorer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/smithy-go"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
)

// observedRequest captures only the bounded labels exposed by Observer.
type observedRequest struct{ operation, status string }

// observedRetry captures bounded retry labels.
type observedRetry struct{ operation, reason string }

// recordingObserver records instrumentation callbacks for assertions.
type recordingObserver struct {
	requests []observedRequest
	retries  []observedRetry
}

// ObserveRequest records one logical Cost Explorer operation.
func (observer *recordingObserver) ObserveRequest(operation, status string, _ time.Duration) {
	observer.requests = append(observer.requests, observedRequest{operation, status})
}

// ObserveRetry records one SDK retry decision.
func (observer *recordingObserver) ObserveRetry(operation, reason string) {
	observer.retries = append(observer.retries, observedRetry{operation, reason})
}

// ObservePaginationPage records one pagination page read.
func (observer *recordingObserver) ObservePaginationPage(string) {}

// fakeAPI exercises decorator behavior without network access.
type fakeAPI struct {
	calls        int
	attempts     int
	attemptCount int
	err          error
	retryErr     error
}

// GetCostAndUsage applies operation options and returns the configured result.
func (api *fakeAPI) GetCostAndUsage(
	ctx context.Context,
	_ *awscostexplorer.GetCostAndUsageInput,
	options ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetCostAndUsageOutput, error) {
	api.calls++
	value := awscostexplorer.Options{Retryer: aws.NopRetryer{}}
	for _, option := range options {
		option(&value)
	}
	attemptCount := max(1, api.attemptCount)
	for attempt := 0; attempt < attemptCount; attempt++ {
		release, err := value.Retryer.(aws.RetryerV2).GetAttemptToken(ctx)
		if err != nil {
			return nil, err
		}
		api.attempts++
		if release != nil {
			_ = release(nil)
		}
		if api.retryErr != nil && attempt+1 < attemptCount {
			if _, err := value.Retryer.GetRetryToken(ctx, api.retryErr); err != nil {
				return nil, err
			}
		}
	}
	return &awscostexplorer.GetCostAndUsageOutput{}, api.err
}

// GetCostForecast returns the configured result.
func (api *fakeAPI) GetCostForecast(
	ctx context.Context,
	_ *awscostexplorer.GetCostForecastInput,
	options ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetCostForecastOutput, error) {
	api.calls++
	value := awscostexplorer.Options{Retryer: aws.NopRetryer{}}
	for _, option := range options {
		option(&value)
	}
	release, err := value.Retryer.(aws.RetryerV2).GetAttemptToken(ctx)
	if err != nil {
		return nil, err
	}
	api.attempts++
	if release != nil {
		_ = release(nil)
	}
	return &awscostexplorer.GetCostForecastOutput{}, api.err
}

// TestInstrumentedClientEmitsBoundedLabels verifies request and retry
// observations never contain unbounded error text.
func TestInstrumentedClientEmitsBoundedLabels(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{
		attemptCount: 2,
		err:          errors.New("private failure detail"),
		retryErr: &smithy.GenericAPIError{
			Code: "LimitExceededException", Message: "private retry detail",
		},
	}
	observer := &recordingObserver{}
	client, err := NewInstrumented(api, config.RateLimitConfig{
		RequestsPerSecond: 1, Burst: 5,
	}, observer)
	if err != nil {
		t.Fatalf("NewInstrumented() returned an unexpected error: %v", err)
	}

	_, _ = client.GetCostAndUsage(context.Background(), &awscostexplorer.GetCostAndUsageInput{})
	if len(observer.requests) != 1 ||
		observer.requests[0] != (observedRequest{"GetCostAndUsage", "error"}) {
		t.Fatalf("request observations = %#v", observer.requests)
	}
	if api.attempts != 2 || len(observer.retries) != 1 ||
		observer.retries[0] != (observedRetry{"GetCostAndUsage", "throttle"}) {
		t.Fatalf("retry observations = %#v", observer.retries)
	}
}

func TestInstrumentedClientReportsThrottleRequestStatus(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{
		err: &smithy.GenericAPIError{Code: "ThrottlingException", Message: "private"},
	}
	observer := &recordingObserver{}
	client, err := NewInstrumented(api, config.RateLimitConfig{RequestsPerSecond: 1, Burst: 5}, observer)
	if err != nil {
		t.Fatalf("NewInstrumented() error = %v", err)
	}
	_, _ = client.GetCostAndUsage(context.Background(), &awscostexplorer.GetCostAndUsageInput{})
	if len(observer.requests) != 1 || observer.requests[0].status != "throttle" {
		t.Fatalf("request observations = %#v, want throttle status", observer.requests)
	}
}

// TestInstrumentedClientHonorsCancellationBeforeAPICall verifies waiting for a
// token exits immediately when the request context is canceled.
func TestInstrumentedClientHonorsCancellationBeforeAPICall(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{}
	client, err := NewInstrumented(api, config.RateLimitConfig{
		RequestsPerSecond: 1, Burst: 1,
	}, &recordingObserver{})
	if err != nil {
		t.Fatalf("NewInstrumented() returned an unexpected error: %v", err)
	}
	_, _ = client.GetCostAndUsage(context.Background(), &awscostexplorer.GetCostAndUsageInput{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.GetCostAndUsage(ctx, &awscostexplorer.GetCostAndUsageInput{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetCostAndUsage() error = %v, want context.Canceled", err)
	}
	if api.calls != 2 || api.attempts != 1 {
		t.Fatalf("logical calls=%d attempts=%d, want 2 logical calls and 1 attempt", api.calls, api.attempts)
	}
}

type countingLimiter struct {
	calls int
	err   error
}

func (limiter *countingLimiter) Wait(context.Context) error {
	limiter.calls++
	return limiter.err
}

type failingAttemptRetryer struct{ aws.NopRetryer }

func (failingAttemptRetryer) GetAttemptToken(context.Context) (func(error) error, error) {
	return nil, errors.New("underlying attempt token failure")
}

func TestInstrumentedClientLimitsEveryAttempt(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{attemptCount: 3, retryErr: errors.New("retry")}
	limiter := &countingLimiter{}
	client, err := newInstrumentedClient(api, limiter, &recordingObserver{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetCostAndUsage(context.Background(), &awscostexplorer.GetCostAndUsageInput{}); err != nil {
		t.Fatal(err)
	}
	if limiter.calls != 3 || api.attempts != 3 {
		t.Fatalf("limiter calls=%d attempts=%d, want 3 each", limiter.calls, api.attempts)
	}
}

func TestInstrumentedClientLimitsForecastAttempt(t *testing.T) {
	t.Parallel()

	api, limiter := &fakeAPI{}, &countingLimiter{}
	client, err := newInstrumentedClient(api, limiter, &recordingObserver{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetCostForecast(context.Background(), &awscostexplorer.GetCostForecastInput{}); err != nil {
		t.Fatal(err)
	}
	if limiter.calls != 1 || api.attempts != 1 {
		t.Fatalf("limiter calls=%d attempts=%d, want 1 each", limiter.calls, api.attempts)
	}
}

func TestRetryObserverReturnsLimiterAndUnderlyingTokenFailures(t *testing.T) {
	t.Parallel()

	limitErr := context.Canceled
	limiter := &countingLimiter{err: limitErr}
	observer := retryObserver{Retryer: failingAttemptRetryer{}, limiter: limiter, observer: discardObserver{}}
	if _, err := observer.GetAttemptToken(context.Background()); !errors.Is(err, limitErr) {
		t.Fatalf("limiter error = %v, want context.Canceled", err)
	}
	limiter.err = nil
	if _, err := observer.GetAttemptToken(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "underlying attempt token failure") {
		t.Fatalf("underlying token error = %v", err)
	}
}
