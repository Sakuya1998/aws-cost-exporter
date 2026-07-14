// Package costexplorer constructs AWS Cost Explorer infrastructure clients.
package costexplorer

import (
	"context"
	"fmt"
	rand "math/rand/v2"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"

	appconfig "github.com/sakuya1998/aws-cost-exporter/internal/config"
)

// New constructs a Cost Explorer client from the AWS default credential chain.
func New(ctx context.Context, value appconfig.AWSConfig) (*awscostexplorer.Client, error) {
	sdkConfig, err := newSDKConfig(ctx, value)
	if err != nil {
		return nil, fmt.Errorf("load AWS SDK config: %w", err)
	}

	return awscostexplorer.NewFromConfig(sdkConfig, func(options *awscostexplorer.Options) {
		options.AppID = "aws-cost-exporter"
		if strings.TrimSpace(value.EndpointURL) != "" {
			options.BaseEndpoint = aws.String(strings.TrimSpace(value.EndpointURL))
		}
	}), nil
}

// newSDKConfig applies client-level timeout, profile, region, and retry policy.
func newSDKConfig(ctx context.Context, value appconfig.AWSConfig) (aws.Config, error) {
	httpClient := awshttp.NewBuildableClient().WithTimeout(value.RequestTimeout)
	options := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(value.Region),
		awsconfig.WithHTTPClient(httpClient),
		awsconfig.WithRetryer(func() aws.Retryer {
			return newRetryer(value.Retry)
		}),
	}
	if strings.TrimSpace(value.Profile) != "" {
		options = append(options, awsconfig.WithSharedConfigProfile(strings.TrimSpace(value.Profile)))
	}

	return awsconfig.LoadDefaultConfig(ctx, options...)
}

// newRetryer creates the SDK standard retryer with project backoff settings.
func newRetryer(value appconfig.RetryConfig) aws.Retryer {
	return retry.NewStandard(func(options *retry.StandardOptions) {
		options.MaxAttempts = value.MaxAttempts
		options.MaxBackoff = value.MaxBackoff
		options.Backoff = jitterBackoff{base: value.BaseDelay, max: value.MaxBackoff}
	})
}

// jitterBackoff implements capped exponential full jitter.
type jitterBackoff struct {
	base time.Duration
	max  time.Duration
}

// BackoffDelay returns a randomized delay bounded by the configured maximum.
func (backoff jitterBackoff) BackoffDelay(attempt int, _ error) (time.Duration, error) {
	delay := backoff.base
	for step := 1; step < attempt && delay < backoff.max; step++ {
		if delay > backoff.max/2 {
			delay = backoff.max
			break
		}
		delay *= 2
	}
	if delay > backoff.max {
		delay = backoff.max
	}

	return time.Duration(rand.Float64() * float64(delay)), nil // #nosec G404 -- retry jitter is not security-sensitive randomness.
}
