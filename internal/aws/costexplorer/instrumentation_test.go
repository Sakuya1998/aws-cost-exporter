package costexplorer

import (
	"context"
	"errors"
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

// fakeAPI exercises decorator behavior without network access.
type fakeAPI struct {
	calls    int
	err      error
	retryErr error
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
	if api.retryErr != nil {
		_, _ = value.Retryer.GetRetryToken(ctx, api.retryErr)
	}
	return &awscostexplorer.GetCostAndUsageOutput{}, api.err
}

// GetCostForecast returns the configured result.
func (api *fakeAPI) GetCostForecast(
	context.Context,
	*awscostexplorer.GetCostForecastInput,
	...func(*awscostexplorer.Options),
) (*awscostexplorer.GetCostForecastOutput, error) {
	api.calls++
	return &awscostexplorer.GetCostForecastOutput{}, api.err
}

// TestInstrumentedClientEmitsBoundedLabels verifies request and retry
// observations never contain unbounded error text.
func TestInstrumentedClientEmitsBoundedLabels(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{
		err: errors.New("private failure detail"),
		retryErr: &smithy.GenericAPIError{
			Code: "LimitExceededException", Message: "private retry detail",
		},
	}
	observer := &recordingObserver{}
	client, err := NewInstrumented(api, config.RateLimitConfig{
		RequestsPerSecond: 1000, Burst: 1,
	}, observer)
	if err != nil {
		t.Fatalf("NewInstrumented() returned an unexpected error: %v", err)
	}

	_, _ = client.GetCostAndUsage(context.Background(), &awscostexplorer.GetCostAndUsageInput{})
	if len(observer.requests) != 1 ||
		observer.requests[0] != (observedRequest{"GetCostAndUsage", "error"}) {
		t.Fatalf("request observations = %#v", observer.requests)
	}
	if len(observer.retries) != 1 ||
		observer.retries[0] != (observedRetry{"GetCostAndUsage", "throttle"}) {
		t.Fatalf("retry observations = %#v", observer.retries)
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
	if api.calls != 1 {
		t.Fatalf("API calls = %d, want 1", api.calls)
	}
}
