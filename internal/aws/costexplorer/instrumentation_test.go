package costexplorer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/smithy-go"

	awscommon "github.com/sakuya1998/aws-cost-exporter/internal/aws/common"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
)

type observedRequest struct {
	target            identity.TargetID
	operation, status string
}
type observedRetry struct {
	target            identity.TargetID
	operation, reason string
}
type recordingObserver struct {
	requests []observedRequest
	retries  []observedRetry
	pages    int
}

func (value *recordingObserver) ObserveRequest(target identity.TargetID, operation, status string, _ time.Duration) {
	value.requests = append(value.requests, observedRequest{target, operation, status})
}
func (value *recordingObserver) ObserveRetry(target identity.TargetID, operation, reason string) {
	value.retries = append(value.retries, observedRetry{target, operation, reason})
}
func (value *recordingObserver) ObservePaginationPage(identity.TargetID, string) { value.pages++ }

type fakeAPI struct {
	err                           error
	usageOptions, forecastOptions awscostexplorer.Options
}

func (value *fakeAPI) GetCostAndUsage(_ context.Context, _ *awscostexplorer.GetCostAndUsageInput, options ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostAndUsageOutput, error) {
	value.usageOptions = awscostexplorer.Options{Retryer: aws.NopRetryer{}}
	for _, option := range options {
		option(&value.usageOptions)
	}
	return &awscostexplorer.GetCostAndUsageOutput{}, value.err
}
func (value *fakeAPI) GetCostForecast(_ context.Context, _ *awscostexplorer.GetCostForecastInput, options ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostForecastOutput, error) {
	value.forecastOptions = awscostexplorer.Options{Retryer: aws.NopRetryer{}}
	for _, option := range options {
		option(&value.forecastOptions)
	}
	return &awscostexplorer.GetCostForecastOutput{}, value.err
}

func TestInstrumentedClientEmitsTargetBoundedLogicalOperations(t *testing.T) {
	api := &fakeAPI{err: &smithy.GenericAPIError{Code: "ThrottlingException", Message: "private"}}
	observer := &recordingObserver{}
	operations := make([]string, 0, 2)
	client, err := NewInstrumented("payer-prod", api, observer, func(operation string) aws.Retryer {
		operations = append(operations, operation)
		return aws.NopRetryer{}
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = client.GetCostAndUsage(context.Background(), &awscostexplorer.GetCostAndUsageInput{})
	_, _ = client.GetCostForecast(context.Background(), &awscostexplorer.GetCostForecastInput{})
	if len(observer.requests) != 2 || observer.requests[0] != (observedRequest{"payer-prod", "GetCostAndUsage", "throttle"}) || observer.requests[1].operation != "GetCostForecast" {
		t.Fatalf("requests=%#v", observer.requests)
	}
	if len(operations) != 2 || operations[0] != "GetCostAndUsage" || operations[1] != "GetCostForecast" {
		t.Fatalf("retryer operations=%v", operations)
	}
}

func TestInstrumentedClientReportsCancellationAndValidation(t *testing.T) {
	api := &fakeAPI{err: context.Canceled}
	observer := &recordingObserver{}
	client, _ := NewInstrumented("a", api, observer)
	_, _ = client.GetCostAndUsage(context.Background(), &awscostexplorer.GetCostAndUsageInput{})
	if observer.requests[0].status != "canceled" {
		t.Fatalf("status=%s", observer.requests[0].status)
	}
	if client, err := NewInstrumented("", api, observer); client != nil || err == nil {
		t.Fatal("accepted empty target")
	}
	if got := retryReason(&timeoutError{}); got != "timeout" {
		t.Fatalf("retryReason=%s", got)
	}
	if got := requestStatus(errors.New("private")); got != "error" {
		t.Fatalf("requestStatus=%s", got)
	}
}

type timeoutError struct{}

func (*timeoutError) Error() string   { return "timeout" }
func (*timeoutError) Timeout() bool   { return true }
func (*timeoutError) Temporary() bool { return true }

var _ awscommon.Observer = (*recordingObserver)(nil)
