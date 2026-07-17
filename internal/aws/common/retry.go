package common

import (
	rand "math/rand/v2"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
)

// NewRetryer creates the SDK standard retryer with project backoff settings.
func NewRetryer(value config.RetryConfig) aws.Retryer {
	return retry.NewStandard(func(options *retry.StandardOptions) {
		options.MaxAttempts = value.MaxAttempts
		options.MaxBackoff = value.MaxBackoff
		options.Backoff = jitterBackoff{base: value.BaseDelay, max: value.MaxBackoff}
	})
}

type jitterBackoff struct{ base, max time.Duration }

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
	return time.Duration(rand.Float64() * float64(delay)), nil // #nosec G404 -- retry jitter is not security-sensitive.
}
