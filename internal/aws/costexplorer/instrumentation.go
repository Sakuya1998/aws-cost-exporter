package costexplorer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/smithy-go"
	"golang.org/x/time/rate"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
)

const (
	operationCostAndUsage = "GetCostAndUsage"
	operationCostForecast = "GetCostForecast"
)

// API is the Cost Explorer SDK surface consumed by adapters.
type API interface {
	GetCostAndUsage(context.Context, *awscostexplorer.GetCostAndUsageInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostAndUsageOutput, error)
	GetCostForecast(context.Context, *awscostexplorer.GetCostForecastInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostForecastOutput, error)
}

// Observer receives only bounded operation, status, and retry-reason labels.
// Implementations must be safe for concurrent use.
type Observer interface {
	ObserveRequest(operation, status string, duration time.Duration)
	ObserveRetry(operation, reason string)
	ObservePaginationPage(operation string)
}

// InstrumentedClient applies global rate limiting and bounded observations.
type InstrumentedClient struct {
	api      API
	limiter  *rate.Limiter
	observer Observer
}

// NewInstrumented decorates a Cost Explorer API client.
func NewInstrumented(api API, value config.RateLimitConfig, observer Observer) (*InstrumentedClient, error) {
	if api == nil {
		return nil, errors.New("cost explorer API must not be nil")
	}
	if value.RequestsPerSecond <= 0 || value.Burst <= 0 {
		return nil, errors.New("rate limit must be positive")
	}
	if observer == nil {
		observer = discardObserver{}
	}

	return &InstrumentedClient{
		api:      api,
		limiter:  rate.NewLimiter(rate.Limit(value.RequestsPerSecond), value.Burst),
		observer: observer,
	}, nil
}

// GetCostAndUsage rate-limits and observes one paginated cost operation.
func (client *InstrumentedClient) GetCostAndUsage(
	ctx context.Context,
	input *awscostexplorer.GetCostAndUsageInput,
	options ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetCostAndUsageOutput, error) {
	started := time.Now()
	if err := client.limiter.Wait(ctx); err != nil {
		client.observer.ObserveRequest(operationCostAndUsage, requestStatus(err), time.Since(started))
		return nil, fmt.Errorf("wait for Cost Explorer rate limit: %w", err)
	}

	output, err := client.api.GetCostAndUsage(
		ctx, input, client.withRetryObserver(operationCostAndUsage, options)...,
	)
	client.observer.ObserveRequest(operationCostAndUsage, requestStatus(err), time.Since(started))

	return output, err
}

// GetCostForecast rate-limits and observes one forecast operation.
func (client *InstrumentedClient) GetCostForecast(
	ctx context.Context,
	input *awscostexplorer.GetCostForecastInput,
	options ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetCostForecastOutput, error) {
	started := time.Now()
	if err := client.limiter.Wait(ctx); err != nil {
		client.observer.ObserveRequest(operationCostForecast, requestStatus(err), time.Since(started))
		return nil, fmt.Errorf("wait for Cost Explorer rate limit: %w", err)
	}

	output, err := client.api.GetCostForecast(
		ctx, input, client.withRetryObserver(operationCostForecast, options)...,
	)
	client.observer.ObserveRequest(operationCostForecast, requestStatus(err), time.Since(started))

	return output, err
}

// withRetryObserver wraps the final per-operation retryer.
func (client *InstrumentedClient) withRetryObserver(operation string, options []func(*awscostexplorer.Options)) []func(*awscostexplorer.Options) {
	result := append([]func(*awscostexplorer.Options){}, options...)

	return append(result, func(value *awscostexplorer.Options) {
		if value.Retryer != nil {
			value.Retryer = retryObserver{
				Retryer: value.Retryer, operation: operation, observer: client.observer,
			}
		}
	})
}

// retryObserver delegates retry policy while recording acquired retry tokens.
type retryObserver struct {
	aws.Retryer
	operation string
	observer  Observer
}

// GetRetryToken observes a retry only after the SDK grants its token.
func (observer retryObserver) GetRetryToken(ctx context.Context, operationError error) (func(error) error, error) {
	release, err := observer.Retryer.GetRetryToken(ctx, operationError)
	if err == nil {
		observer.observer.ObserveRetry(observer.operation, retryReason(operationError))
	}
	return release, err
}

// GetAttemptToken preserves RetryerV2 attempt-rate behavior when available.
func (observer retryObserver) GetAttemptToken(ctx context.Context) (func(error) error, error) {
	if retryer, ok := observer.Retryer.(aws.RetryerV2); ok {
		return retryer.GetAttemptToken(ctx)
	}
	return observer.GetInitialToken(), nil
}

// requestStatus converts arbitrary failures to bounded status labels.
func requestStatus(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "canceled"
	}
	var apiError smithy.APIError
	if errors.As(err, &apiError) && isThrottleCode(apiError.ErrorCode()) {
		return "throttle"
	}
	return "error"
}

// retryReason converts arbitrary retry errors to bounded reason labels.
func retryReason(err error) string {
	var apiError smithy.APIError
	if errors.As(err, &apiError) && isThrottleCode(apiError.ErrorCode()) {
		return "throttle"
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "timeout"
	}
	return "other"
}

// discardObserver provides zero-allocation callbacks when telemetry is absent.
type discardObserver struct{}

// ObserveRequest discards a request observation.
func (discardObserver) ObserveRequest(string, string, time.Duration) {}

// ObserveRetry discards a retry observation.
func (discardObserver) ObserveRetry(string, string) {}

// ObservePaginationPage discards a pagination observation.
func (discardObserver) ObservePaginationPage(string) {}
