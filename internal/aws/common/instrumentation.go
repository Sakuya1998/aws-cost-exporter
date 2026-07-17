// Package common centralizes bounded AWS SDK instrumentation and attempt policy.
package common

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/smithy-go"
	"golang.org/x/time/rate"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
)

const (
	OperationAssumeRole           = "AssumeRole"
	OperationGetCallerIdentity    = "GetCallerIdentity"
	OperationGetCostAndUsage      = "GetCostAndUsage"
	OperationGetCostForecast      = "GetCostForecast"
	OperationListAccounts         = "ListAccounts"
	OperationDescribeOrganization = "DescribeOrganization"
	OperationDescribeBudgets      = "DescribeBudgets"
)

// Observer receives bounded target, operation, status, and retry labels.
type Observer interface {
	ObserveRequest(identity.TargetID, string, string, time.Duration)
	ObserveRetry(identity.TargetID, string, string)
	ObservePaginationPage(identity.TargetID, string)
}

// DiscardObserver supplies no-op callbacks for adapters without telemetry.
type DiscardObserver struct{}

func (DiscardObserver) ObserveRequest(identity.TargetID, string, string, time.Duration) {}
func (DiscardObserver) ObserveRetry(identity.TargetID, string, string)                  {}
func (DiscardObserver) ObservePaginationPage(identity.TargetID, string)                 {}

// Limiter is the context-aware attempt limiter surface.
type Limiter interface{ Wait(context.Context) error }

// DualLimiter enforces process-wide policy before target-specific policy.
type DualLimiter struct{ Global, Target Limiter }

// Wait obtains both attempt tokens and stops immediately on cancellation.
func (limiter DualLimiter) Wait(ctx context.Context) error {
	if limiter.Global == nil || limiter.Target == nil {
		return errors.New("AWS attempt limiters must not be nil")
	}
	if err := limiter.Global.Wait(ctx); err != nil {
		return fmt.Errorf("wait for global AWS attempt limit: %w", err)
	}
	if err := limiter.Target.Wait(ctx); err != nil {
		return fmt.Errorf("wait for target AWS attempt limit: %w", err)
	}
	return nil
}

// NewLimiter constructs one token-bucket limiter.
func NewLimiter(requestsPerSecond float64, burst int) Limiter {
	return rate.NewLimiter(rate.Limit(requestsPerSecond), burst)
}

// WrapRetryer preserves SDK retry behavior while limiting every attempt.
func WrapRetryer(base aws.Retryer, target identity.TargetID, operation string, limiter Limiter, observer Observer) aws.Retryer {
	if observer == nil {
		observer = DiscardObserver{}
	}
	return retryObserver{Retryer: base, target: target, operation: operation, limiter: limiter, observer: observer}
}

type retryObserver struct {
	aws.Retryer
	target    identity.TargetID
	operation string
	limiter   Limiter
	observer  Observer
}

func (observer retryObserver) GetRetryToken(ctx context.Context, operationError error) (func(error) error, error) {
	release, err := observer.Retryer.GetRetryToken(ctx, operationError)
	if err == nil {
		observer.observer.ObserveRetry(observer.target, observer.operation, RetryReason(operationError))
	}
	return release, err
}

func (observer retryObserver) GetAttemptToken(ctx context.Context) (func(error) error, error) {
	if observer.limiter == nil {
		return nil, errors.New("AWS attempt limiter must not be nil")
	}
	if err := observer.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	if retryer, ok := observer.Retryer.(aws.RetryerV2); ok {
		return retryer.GetAttemptToken(ctx)
	}
	return observer.GetInitialToken(), nil
}

// ObserveCall records one logical SDK operation around all retries.
func ObserveCall(observer Observer, target identity.TargetID, operation string, started time.Time, err error) {
	if observer != nil {
		observer.ObserveRequest(target, operation, RequestStatus(err), time.Since(started))
	}
}

// RequestStatus converts arbitrary failures to bounded metric values.
func RequestStatus(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "canceled"
	}
	var apiError smithy.APIError
	if errors.As(err, &apiError) && IsThrottleCode(apiError.ErrorCode()) {
		return "throttle"
	}
	return "error"
}

// RetryReason converts arbitrary failures to bounded metric values.
func RetryReason(err error) string {
	var apiError smithy.APIError
	if errors.As(err, &apiError) && IsThrottleCode(apiError.ErrorCode()) {
		return "throttle"
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "timeout"
	}
	return "other"
}

// IsThrottleCode recognizes stable AWS throttling error codes.
func IsThrottleCode(code string) bool {
	switch code {
	case "Throttling", "ThrottlingException", "TooManyRequestsException", "RequestLimitExceeded", "LimitExceededException":
		return true
	default:
		return false
	}
}
