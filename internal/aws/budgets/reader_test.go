package budgets

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsbudgets "github.com/aws/aws-sdk-go-v2/service/budgets"
	budgettypes "github.com/aws/aws-sdk-go-v2/service/budgets/types"
)

type fakeBudgets struct {
	calls   int
	missing bool
}

func (value *fakeBudgets) DescribeBudgets(_ context.Context, input *awsbudgets.DescribeBudgetsInput, _ ...func(*awsbudgets.Options)) (*awsbudgets.DescribeBudgetsOutput, error) {
	value.calls++
	if input.NextToken == nil {
		return &awsbudgets.DescribeBudgetsOutput{Budgets: []budgettypes.Budget{{BudgetName: aws.String("Ignored"), BudgetType: budgettypes.BudgetTypeCost, TimeUnit: budgettypes.TimeUnitMonthly, BudgetLimit: &budgettypes.Spend{Amount: aws.String("1"), Unit: aws.String("USD")}}}, NextToken: aws.String("next")}, nil
	}
	if value.missing {
		return &awsbudgets.DescribeBudgetsOutput{}, nil
	}
	return &awsbudgets.DescribeBudgetsOutput{Budgets: []budgettypes.Budget{{BudgetName: aws.String("Monthly"), BudgetType: budgettypes.BudgetTypeCost, TimeUnit: budgettypes.TimeUnitMonthly, BudgetLimit: &budgettypes.Spend{Amount: aws.String("100"), Unit: aws.String("USD")}, CalculatedSpend: &budgettypes.CalculatedSpend{ActualSpend: &budgettypes.Spend{Amount: aws.String("45"), Unit: aws.String("USD")}}}}}, nil
}

func TestReaderPaginatesAllowlistAndOmitsMissingForecast(t *testing.T) {
	api := &fakeBudgets{}
	reader, err := New("payer", "444455556666", api, 3, []string{"Monthly"}, nil, func(string) aws.Retryer { return aws.NopRetryer{} })
	if err != nil {
		t.Fatal(err)
	}
	values, err := reader.ReadBudgets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if api.calls != 2 || len(values) != 1 || values[0].Limit.Amount() != 100 || !values[0].HasActual || values[0].HasForecasted {
		t.Fatalf("budgets=%#v calls=%d", values, api.calls)
	}
}

func TestReaderRejectsMissingAllowlistedBudget(t *testing.T) {
	reader, _ := New("payer", "444455556666", &fakeBudgets{missing: true}, 3, []string{"Monthly"}, nil, func(string) aws.Retryer { return aws.NopRetryer{} })
	if _, err := reader.ReadBudgets(context.Background()); err == nil {
		t.Fatal("accepted missing allowlisted budget")
	}
}

func TestReaderRejectsNonCostBudgetAndMismatchedUnits(t *testing.T) {
	reader := &Reader{target: "payer"}
	base := budgettypes.Budget{
		BudgetName: aws.String("Usage"), BudgetType: budgettypes.BudgetTypeUsage,
		TimeUnit:    budgettypes.TimeUnitMonthly,
		BudgetLimit: &budgettypes.Spend{Amount: aws.String("100"), Unit: aws.String("GB")},
	}
	if _, err := reader.mapBudget(base); err == nil {
		t.Fatal("accepted non-COST budget")
	}
	base.BudgetType = budgettypes.BudgetTypeCost
	base.BudgetLimit.Unit = aws.String("USD")
	base.CalculatedSpend = &budgettypes.CalculatedSpend{
		ActualSpend: &budgettypes.Spend{Amount: aws.String("1"), Unit: aws.String("EUR")},
	}
	if _, err := reader.mapBudget(base); err == nil {
		t.Fatal("accepted mismatched budget units")
	}
}
