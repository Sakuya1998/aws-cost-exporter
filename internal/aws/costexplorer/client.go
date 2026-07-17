// Package costexplorer constructs AWS Cost Explorer infrastructure clients.
package costexplorer

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"

	awscommon "github.com/sakuya1998/aws-cost-exporter/internal/aws/common"
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
		if strings.TrimSpace(value.Endpoints.CostExplorer) != "" {
			options.BaseEndpoint = aws.String(strings.TrimSpace(value.Endpoints.CostExplorer))
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
			return awscommon.NewRetryer(value.Retry)
		}),
	}
	if strings.TrimSpace(value.Profile) != "" {
		options = append(options, awsconfig.WithSharedConfigProfile(strings.TrimSpace(value.Profile)))
	}

	return awsconfig.LoadDefaultConfig(ctx, options...)
}
