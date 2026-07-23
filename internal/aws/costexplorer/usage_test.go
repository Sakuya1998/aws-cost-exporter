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
	subject, err := NewUsageAdapter(api, 50, metricUnblendedCost, nil)
	if err != nil {
		t.Fatalf("NewUsageAdapter() error = %v", err)
	}
	period, _ := cost.NewPeriod(
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
	)
	query := ports.CostQuery{
		Period: period, Window: cost.WindowMonthToDate, Basis: cost.BasisUnblended, GroupBy: cost.DimensionService,
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

func TestUsageAdapterAggregatesMTDAcrossDailyRows(t *testing.T) {
	api := &multiDayUsageAPI{}
	subject, err := NewUsageAdapter(api, 50, metricUnblendedCost, nil)
	if err != nil {
		t.Fatalf("NewUsageAdapter() error = %v", err)
	}
	period, _ := cost.NewPeriod(
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC),
	)
	query := ports.CostQuery{
		Period: period, Window: cost.WindowMonthToDate, Basis: cost.BasisUnblended, GroupBy: cost.DimensionService,
	}
	values, err := subject.ReadCosts(context.Background(), query)
	if err != nil {
		t.Fatalf("ReadCosts() error = %v", err)
	}
	if len(values) != 1 || values[0].Dimension.Value() != "Amazon EC2" ||
		values[0].Amount.Amount() != 6 {
		t.Fatalf("ReadCosts() = %#v, want one aggregated 6 USD EC2 MTD cost", values)
	}
}

func TestUsageAdapterReadsAllowlistedTagCostsAndBasisMetrics(t *testing.T) {
	api := &tagUsageAPI{}
	subject, err := NewUsageAdapterForTarget("payer", api, 10, metricUnblendedCost, nil)
	if err != nil {
		t.Fatal(err)
	}
	period := cost.DayContaining(time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC))
	values, err := subject.ReadTagCosts(context.Background(), ports.CostQuery{Period: period, Window: cost.WindowDaily, Basis: cost.BasisNet}, "Environment")
	if err != nil || len(values) != 2 || values[0].TagValue != "prod" || values[1].TagValue != "dev" {
		t.Fatalf("tag values=%#v err=%v", values, err)
	}
	if api.input == nil || api.input.GroupBy[0].Type != cetypes.GroupDefinitionTypeTag || api.input.Metrics[0] != "NetUnblendedCost" {
		t.Fatalf("tag input=%#v", api.input)
	}
	for _, basis := range []cost.Basis{cost.BasisAmortized, cost.BasisNet} {
		if _, err := subject.ReadCosts(context.Background(), ports.CostQuery{Period: period, Window: cost.WindowDaily, Basis: basis, GroupBy: cost.DimensionAccount}); err != nil {
			t.Fatalf("basis %s failed: %v", basis, err)
		}
	}
	bad := &tagUsageAPI{malformed: true}
	adapter, _ := NewUsageAdapterForTarget("payer", bad, 10, metricUnblendedCost, nil)
	if _, err := adapter.ReadTagCosts(context.Background(), ports.CostQuery{Period: period, Window: cost.WindowDaily, Basis: cost.BasisUnblended}, "Environment"); err == nil {
		t.Fatal("accepted malformed tag response")
	}
}

// TestUsageAdapterRejectsPartialFailuresAndInvalidQueries verifies boundaries.
func TestUsageAdapterRejectsPartialFailuresAndInvalidQueries(t *testing.T) {
	if adapter, err := NewUsageAdapter(nil, 50, metricUnblendedCost, nil); adapter != nil || !errors.Is(err, ErrNilUsageAPI) {
		t.Fatalf("NewUsageAdapter(nil) = %#v, %v", adapter, err)
	}
	api := &usageAPI{err: &smithy.GenericAPIError{Code: "ThrottlingException", Message: "private"}}
	subject, _ := NewUsageAdapter(api, 50, metricUnblendedCost, nil)
	query := ports.CostQuery{Period: cost.DayContaining(time.Now()), Basis: cost.BasisUnblended, GroupBy: cost.DimensionTotal}
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

type multiDayUsageAPI struct {
	input *awscostexplorer.GetCostAndUsageInput
}

type tagUsageAPI struct {
	input     *awscostexplorer.GetCostAndUsageInput
	malformed bool
}

func (api *tagUsageAPI) GetCostAndUsage(_ context.Context, input *awscostexplorer.GetCostAndUsageInput, _ ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostAndUsageOutput, error) {
	api.input = input
	metric := input.Metrics[0]
	if api.malformed {
		return &awscostexplorer.GetCostAndUsageOutput{ResultsByTime: []cetypes.ResultByTime{{TimePeriod: input.TimePeriod, Groups: []cetypes.Group{{Keys: []string{"Environment$prod", "extra"}, Metrics: map[string]cetypes.MetricValue{metric: {Amount: aws.String("1"), Unit: aws.String("USD")}}}}}}}, nil
	}
	return &awscostexplorer.GetCostAndUsageOutput{ResultsByTime: []cetypes.ResultByTime{{TimePeriod: input.TimePeriod, Groups: []cetypes.Group{
		{Keys: []string{"Environment$prod"}, Metrics: map[string]cetypes.MetricValue{metric: {Amount: aws.String("2"), Unit: aws.String("USD")}}},
		{Keys: []string{"Environment$dev"}, Metrics: map[string]cetypes.MetricValue{metric: {Amount: aws.String("1"), Unit: aws.String("USD")}}},
	}}}}, nil
}

func (api *tagUsageAPI) GetCostForecast(context.Context, *awscostexplorer.GetCostForecastInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostForecastOutput, error) {
	return nil, errors.New("unexpected forecast call")
}

func (api *multiDayUsageAPI) GetCostAndUsage(_ context.Context, input *awscostexplorer.GetCostAndUsageInput, _ ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostAndUsageOutput, error) {
	api.input = input
	return &awscostexplorer.GetCostAndUsageOutput{ResultsByTime: []cetypes.ResultByTime{
		{
			TimePeriod: &cetypes.DateInterval{Start: aws.String("2026-07-01"), End: aws.String("2026-07-02")},
			Groups: []cetypes.Group{{
				Keys: []string{"Amazon EC2"},
				Metrics: map[string]cetypes.MetricValue{
					"UnblendedCost": {Amount: aws.String("1"), Unit: aws.String("USD")},
				},
			}},
		},
		{
			TimePeriod: &cetypes.DateInterval{Start: aws.String("2026-07-02"), End: aws.String("2026-07-03")},
			Groups: []cetypes.Group{{
				Keys: []string{"Amazon EC2"},
				Metrics: map[string]cetypes.MetricValue{
					"UnblendedCost": {Amount: aws.String("2"), Unit: aws.String("USD")},
				},
			}},
		},
		{
			TimePeriod: &cetypes.DateInterval{Start: aws.String("2026-07-03"), End: aws.String("2026-07-04")},
			Groups: []cetypes.Group{{
				Keys: []string{"Amazon EC2"},
				Metrics: map[string]cetypes.MetricValue{
					"UnblendedCost": {Amount: aws.String("3"), Unit: aws.String("USD")},
				},
			}},
		},
	}}, nil
}

func (*multiDayUsageAPI) GetCostForecast(context.Context, *awscostexplorer.GetCostForecastInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostForecastOutput, error) {
	return nil, nil
}

func (*usageAPI) GetCostForecast(context.Context, *awscostexplorer.GetCostForecastInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostForecastOutput, error) {
	return nil, nil
}
