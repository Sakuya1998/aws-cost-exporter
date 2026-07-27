package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	costexploreradapter "github.com/sakuya1998/aws-cost-exporter/internal/aws/costexplorer"
	commitmentcollector "github.com/sakuya1998/aws-cost-exporter/internal/collector/commitment"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/commitment"
)

func TestCommitmentReaderCollectorPipeline(t *testing.T) {
	reader, err := costexploreradapter.NewCommitmentReader("payer", commitmentPipelineAPI{}, 2, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := commitmentcollector.New("payer", reader, 20)
	if err != nil {
		t.Fatal(err)
	}
	result, err := subject.Collect(context.Background(), time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	values := result.Commitments()
	if len(values) != 2 || values[0].Type != commitment.TypeReservation || values[1].Type != commitment.TypeSavingsPlan || !values[0].HasUtilization || !values[0].HasCoverage || !values[1].HasUtilization || !values[1].HasCoverage {
		t.Fatalf("commitments=%#v", values)
	}
}

type commitmentPipelineAPI struct{}

func (commitmentPipelineAPI) GetSavingsPlansUtilization(context.Context, *awscostexplorer.GetSavingsPlansUtilizationInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetSavingsPlansUtilizationOutput, error) {
	return &awscostexplorer.GetSavingsPlansUtilizationOutput{Total: &cetypes.SavingsPlansUtilizationAggregates{Utilization: &cetypes.SavingsPlansUtilization{UtilizationPercentage: aws.String("80")}, Savings: &cetypes.SavingsPlansSavings{NetSavings: aws.String("4")}}}, nil
}

func (commitmentPipelineAPI) GetSavingsPlansCoverage(context.Context, *awscostexplorer.GetSavingsPlansCoverageInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetSavingsPlansCoverageOutput, error) {
	return &awscostexplorer.GetSavingsPlansCoverageOutput{SavingsPlansCoverages: []cetypes.SavingsPlansCoverage{{Coverage: &cetypes.SavingsPlansCoverageData{SpendCoveredBySavingsPlans: aws.String("75"), OnDemandCost: aws.String("25"), TotalCost: aws.String("100")}}}}, nil
}

func (commitmentPipelineAPI) GetReservationUtilization(context.Context, *awscostexplorer.GetReservationUtilizationInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetReservationUtilizationOutput, error) {
	return &awscostexplorer.GetReservationUtilizationOutput{Total: &cetypes.ReservationAggregates{UtilizationPercentage: aws.String("50"), UnusedHours: aws.String("3"), NetRISavings: aws.String("5")}}, nil
}

func (commitmentPipelineAPI) GetReservationCoverage(context.Context, *awscostexplorer.GetReservationCoverageInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetReservationCoverageOutput, error) {
	return &awscostexplorer.GetReservationCoverageOutput{Total: &cetypes.Coverage{CoverageHours: &cetypes.CoverageHours{CoverageHoursPercentage: aws.String("60")}, CoverageCost: &cetypes.CoverageCost{OnDemandCost: aws.String("40")}}}, nil
}
