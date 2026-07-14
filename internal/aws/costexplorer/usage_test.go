package costexplorer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/smithy-go"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

// TestUsageAdapterSerializesQueriesAndMapsCompletePages verifies the port.
func TestUsageAdapterSerializesQueriesAndMapsCompletePages(t *testing.T) {
	api := &usageAPI{}
	subject, err := NewUsageAdapter(api, 50, nil)
	if err != nil {
		t.Fatalf("NewUsageAdapter() error = %v", err)
	}
	period, _ := cost.NewPeriod(
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
	)
	query := ports.CostQuery{
		Period: period, Window: cost.WindowMonthToDate, GroupBy: cost.DimensionService,
		LinkedAccountIDs: []string{"123456789012"}, Services: []string{"Amazon EC2"},
		Regions: []string{"us-east-1"},
	}
	values, err := subject.ReadCosts(context.Background(), query)
	if err != nil || len(values) != 1 || values[0].Dimension.Value() != "Amazon EC2" ||
		values[0].Amount.Amount() != 12.5 {
		t.Fatalf("ReadCosts() = %#v, %v", values, err)
	}
	input := api.input
	if aws.ToString(input.TimePeriod.Start) != "2026-07-01" ||
		aws.ToString(input.TimePeriod.End) != "2026-07-14" ||
		len(input.Metrics) != 1 || input.Metrics[0] != "UnblendedCost" ||
		len(input.GroupBy) != 1 || aws.ToString(input.GroupBy[0].Key) != "SERVICE" ||
		input.Granularity != cetypes.GranularityDaily ||
		input.GroupBy[0].Type != cetypes.GroupDefinitionTypeDimension ||
		input.Filter == nil || len(input.Filter.And) != 3 {
		t.Fatalf("serialized input = %#v", input)
	}
}

// TestUsageAdapterRejectsPartialFailuresAndInvalidQueries verifies boundaries.
func TestUsageAdapterRejectsPartialFailuresAndInvalidQueries(t *testing.T) {
	if adapter, err := NewUsageAdapter(nil, 50, nil); adapter != nil || !errors.Is(err, ErrNilUsageAPI) {
		t.Fatalf("NewUsageAdapter(nil) = %#v, %v", adapter, err)
	}
	api := &usageAPI{err: &smithy.GenericAPIError{Code: "ThrottlingException", Message: "private"}}
	subject, _ := NewUsageAdapter(api, 50, nil)
	query := ports.CostQuery{Period: cost.DayContaining(time.Now()), GroupBy: cost.DimensionTotal}
	values, err := subject.ReadCosts(context.Background(), query)
	var classified *ClassifiedError
	if values != nil || !errors.As(err, &classified) || !classified.Retryable() {
		t.Fatalf("ReadCosts(failure) = %#v, %v; want retryable and no values", values, err)
	}
	if filter := usageFilter(ports.CostQuery{Services: []string{"EC2"}}); filter == nil ||
		filter.Dimensions == nil || len(filter.And) != 0 {
		t.Fatalf("single filter = %#v, want one dimension expression", filter)
	}
	if usageFilter(ports.CostQuery{}) != nil {
		t.Fatal("empty query produced a filter")
	}
	if _, err := usageDimension(cost.DimensionTotal); err == nil {
		t.Fatal("total unexpectedly accepted as grouped dimension")
	}
}

type usageAPI struct {
	input *awscostexplorer.GetCostAndUsageInput
	err   error
}

func (api *usageAPI) GetCostAndUsage(_ context.Context, input *awscostexplorer.GetCostAndUsageInput, _ ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostAndUsageOutput, error) {
	api.input = input
	if api.err != nil {
		return nil, api.err
	}
	return &awscostexplorer.GetCostAndUsageOutput{ResultsByTime: []cetypes.ResultByTime{{
		TimePeriod: &cetypes.DateInterval{Start: input.TimePeriod.Start, End: input.TimePeriod.End},
		Groups: []cetypes.Group{{
			Keys: []string{"Amazon EC2"},
			Metrics: map[string]cetypes.MetricValue{
				"UnblendedCost": {Amount: aws.String("12.5"), Unit: aws.String("USD")},
			},
		}},
	}}}, nil
}

func (*usageAPI) GetCostForecast(context.Context, *awscostexplorer.GetCostForecastInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostForecastOutput, error) {
	return nil, nil
}
