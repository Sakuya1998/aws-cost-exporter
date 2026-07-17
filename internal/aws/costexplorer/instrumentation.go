package costexplorer

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"

	awscommon "github.com/sakuya1998/aws-cost-exporter/internal/aws/common"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
)

const (
	operationCostAndUsage = awscommon.OperationGetCostAndUsage
	operationCostForecast = awscommon.OperationGetCostForecast
)

type API interface {
	GetCostAndUsage(context.Context, *awscostexplorer.GetCostAndUsageInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostAndUsageOutput, error)
	GetCostForecast(context.Context, *awscostexplorer.GetCostForecastInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostForecastOutput, error)
}

type Observer = awscommon.Observer

type discardObserver struct{}

func (discardObserver) ObserveRequest(identity.TargetID, string, string, time.Duration) {}
func (discardObserver) ObserveRetry(identity.TargetID, string, string)                  {}
func (discardObserver) ObservePaginationPage(identity.TargetID, string)                 {}

// InstrumentedClient records target-scoped logical Cost Explorer operations.
// Attempt limiting and retry observations are installed in the SDK client retryer.
type InstrumentedClient struct {
	target   identity.TargetID
	api      API
	observer Observer
	retryer  func(string) aws.Retryer
}

func NewInstrumented(target identity.TargetID, api API, observer Observer, retryers ...func(string) aws.Retryer) (*InstrumentedClient, error) {
	if target == "" || api == nil {
		return nil, errors.New("target and cost explorer API must not be empty")
	}
	if observer == nil {
		observer = discardObserver{}
	}
	var retryer func(string) aws.Retryer
	if len(retryers) > 0 {
		retryer = retryers[0]
	}
	return &InstrumentedClient{target: target, api: api, observer: observer, retryer: retryer}, nil
}

func (client *InstrumentedClient) GetCostAndUsage(ctx context.Context, input *awscostexplorer.GetCostAndUsageInput, options ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostAndUsageOutput, error) {
	started := time.Now()
	if client.retryer != nil {
		options = append(options, func(value *awscostexplorer.Options) { value.Retryer = client.retryer(operationCostAndUsage) })
	}
	output, err := client.api.GetCostAndUsage(ctx, input, options...)
	awscommon.ObserveCall(client.observer, client.target, operationCostAndUsage, started, err)
	return output, err
}

func (client *InstrumentedClient) GetCostForecast(ctx context.Context, input *awscostexplorer.GetCostForecastInput, options ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostForecastOutput, error) {
	started := time.Now()
	if client.retryer != nil {
		options = append(options, func(value *awscostexplorer.Options) { value.Retryer = client.retryer(operationCostForecast) })
	}
	output, err := client.api.GetCostForecast(ctx, input, options...)
	awscommon.ObserveCall(client.observer, client.target, operationCostForecast, started, err)
	return output, err
}

func requestStatus(err error) string { return awscommon.RequestStatus(err) }
func retryReason(err error) string   { return awscommon.RetryReason(err) }
