// Package costexplorer constructs AWS Cost Explorer infrastructure clients.
package costexplorer

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"

	awscommon "github.com/sakuya1998/aws-cost-exporter/internal/aws/common"
	appconfig "github.com/sakuya1998/aws-cost-exporter/internal/config"
)

// New constructs a Cost Explorer client from the configured single credential source.
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

// newSDKConfig applies one configured credential source and client policy.
func newSDKConfig(ctx context.Context, value appconfig.AWSConfig) (aws.Config, error) {
	httpClient := awshttp.NewBuildableClient().WithTimeout(value.RequestTimeout)
	options := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(value.Region),
		awsconfig.WithHTTPClient(httpClient),
		awsconfig.WithRetryer(func() aws.Retryer {
			return awscommon.NewRetryer(value.Retry)
		}),
	}
	if len(value.Credentials.Sources) > 1 {
		return aws.Config{}, fmt.Errorf("standalone Cost Explorer client requires exactly one credential source")
	}
	for _, source := range value.Credentials.Sources {
		switch source.Type {
		case appconfig.CredentialSourceProfile:
			options = append(options, awsconfig.WithSharedConfigProfile(source.Profile))
		case appconfig.CredentialSourceStaticEnv:
			options = append(options, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				os.Getenv(source.AccessKeyIDEnv), os.Getenv(source.SecretAccessKeyEnv), os.Getenv(source.SessionTokenEnv),
			)))
		}
	}

	return awsconfig.LoadDefaultConfig(ctx, options...)
}
