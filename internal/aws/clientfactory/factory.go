// Package clientfactory constructs isolated AWS clients for explicit targets.
package clientfactory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	awscommon "github.com/sakuya1998/aws-cost-exporter/internal/aws/common"
	appconfig "github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
)

// Clients contains the service clients belonging to one target credential boundary.
type Clients struct {
	CostExplorer  *costexplorer.Client
	Organizations *organizations.Client
	Budgets       *budgets.Client
	Retryer       func(string) aws.Retryer
}

// Factory loads base credentials once and creates target-scoped clients.
type Factory struct {
	base     aws.Config
	config   appconfig.AWSConfig
	global   awscommon.Limiter
	observer awscommon.Observer
}

// New loads the AWS default credential chain once.
func New(ctx context.Context, value appconfig.AWSConfig, observer awscommon.Observer) (*Factory, error) {
	httpClient := awshttp.NewBuildableClient().WithTimeout(value.RequestTimeout)
	options := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(value.Region), awsconfig.WithHTTPClient(httpClient),
		awsconfig.WithRetryer(func() aws.Retryer { return awscommon.NewRetryer(value.Retry) }),
	}
	if strings.TrimSpace(value.Profile) != "" {
		options = append(options, awsconfig.WithSharedConfigProfile(strings.TrimSpace(value.Profile)))
	}
	base, err := awsconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("load AWS SDK config: %w", err)
	}
	return &Factory{
		base: base, config: value,
		global:   awscommon.NewLimiter(value.RateLimit.GlobalRequestsPerSecond, value.RateLimit.GlobalBurst),
		observer: observer,
	}, nil
}

// ForTarget constructs clients without making a network request.
func (factory *Factory) ForTarget(target appconfig.TargetConfig) (Clients, error) {
	if factory == nil || target.Name == "" {
		return Clients{}, errors.New("invalid AWS client factory target")
	}
	targetID := identity.TargetID(target.Name)
	limiter := awscommon.DualLimiter{
		Global: factory.global,
		Target: awscommon.NewLimiter(factory.config.RateLimit.TargetRequestsPerSecond, factory.config.RateLimit.TargetBurst),
	}
	retryers := make(map[string]aws.Retryer, 6)
	for _, operation := range []string{
		awscommon.OperationAssumeRole, awscommon.OperationGetCostAndUsage,
		awscommon.OperationGetCostForecast, awscommon.OperationListAccounts,
		awscommon.OperationDescribeOrganization, awscommon.OperationDescribeBudgets,
	} {
		retryers[operation] = awscommon.WrapRetryer(factory.base.Retryer(), targetID, operation, limiter, factory.observer)
	}
	retryerFor := func(operation string) aws.Retryer { return retryers[operation] }
	sdkConfig := factory.base.Copy()
	if target.AssumeRole != nil {
		stsClient := sts.NewFromConfig(factory.base, func(options *sts.Options) {
			options.Retryer = retryerFor(awscommon.OperationAssumeRole)
			if endpoint := strings.TrimSpace(factory.config.Endpoints.STS); endpoint != "" {
				options.BaseEndpoint = aws.String(endpoint)
			}
		})
		provider := stscreds.NewAssumeRoleProvider(observedSTS{client: stsClient, target: targetID, observer: factory.observer}, target.AssumeRole.RoleARN, func(options *stscreds.AssumeRoleOptions) {
			externalID := os.Getenv(target.AssumeRole.ExternalIDEnv)
			options.ExternalID = aws.String(externalID)
			options.RoleSessionName = sessionName(targetID, target.AssumeRole.SessionName)
		})
		sdkConfig.Credentials = aws.NewCredentialsCache(provider)
	}
	clients := Clients{}
	clients.Retryer = retryerFor
	clients.CostExplorer = costexplorer.NewFromConfig(sdkConfig, func(options *costexplorer.Options) {
		options.AppID = "aws-cost-exporter"
		options.Retryer = clients.Retryer(awscommon.OperationGetCostAndUsage)
		if endpoint := strings.TrimSpace(factory.config.Endpoints.CostExplorer); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
	clients.Organizations = organizations.NewFromConfig(sdkConfig, func(options *organizations.Options) {
		options.AppID = "aws-cost-exporter"
		options.Retryer = clients.Retryer(awscommon.OperationListAccounts)
		if endpoint := strings.TrimSpace(factory.config.Endpoints.Organizations); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
	clients.Budgets = budgets.NewFromConfig(sdkConfig, func(options *budgets.Options) {
		options.AppID = "aws-cost-exporter"
		options.Retryer = clients.Retryer(awscommon.OperationDescribeBudgets)
		if endpoint := strings.TrimSpace(factory.config.Endpoints.Budgets); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
	return clients, nil
}

func sessionName(target identity.TargetID, configured string) string {
	if configured != "" {
		return configured
	}
	value := "aws-cost-exporter-" + string(target)
	if len(value) > 64 {
		return value[:64]
	}
	return value
}

type observedSTS struct {
	client   *sts.Client
	target   identity.TargetID
	observer awscommon.Observer
}

func (client observedSTS) AssumeRole(ctx context.Context, input *sts.AssumeRoleInput, options ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
	started := time.Now()
	output, err := client.client.AssumeRole(ctx, input, options...)
	awscommon.ObserveCall(client.observer, client.target, awscommon.OperationAssumeRole, started, err)
	return output, err
}
