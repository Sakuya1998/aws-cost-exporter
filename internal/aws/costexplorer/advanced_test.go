package costexplorer

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	"github.com/sakuya1998/aws-cost-exporter/internal/aws/common"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/commitment"
)

func TestCommitmentReaderMapsBoundedSavingsAndReservationSummaries(t *testing.T) {
	reader, err := NewCommitmentReader("payer", commitmentStub{}, 2, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	reference := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	savings, err := reader.ReadSavingsPlans(context.Background(), reference)
	if err != nil {
		t.Fatal(err)
	}
	if savings.Type != commitment.TypeSavingsPlan || savings.UtilizationRatio != .8 || savings.CoverageRatio != .75 || savings.NetSavings.Amount() != 4 || savings.CoveredSpend.Amount() != 75 || savings.HasUnusedHours {
		t.Fatalf("savings=%#v", savings)
	}
	reservation, err := reader.ReadReservations(context.Background(), reference)
	if err != nil {
		t.Fatal(err)
	}
	if reservation.Type != commitment.TypeReservation || reservation.UtilizationRatio != .5 || reservation.CoverageRatio != .6 || reservation.UnusedHours != 3 || reservation.OnDemandCost.Amount() != 40 {
		t.Fatalf("reservation=%#v", reservation)
	}
	for _, call := range []func(context.Context, time.Time) (commitment.Summary, error){reader.ReadSavingsPlansUtilization, reader.ReadSavingsPlansCoverage, reader.ReadReservationUtilization, reader.ReadReservationCoverage} {
		if _, err := call(context.Background(), reference); err != nil {
			t.Fatal(err)
		}
	}
	if value, err := NewCommitmentReader("payer", nil, 1, nil, nil); value != nil || err == nil {
		t.Fatal("accepted nil API")
	}
}

func TestCommitmentReaderPaginatesReservationsAndAttributesTelemetry(t *testing.T) {
	api := &pagedCommitmentStub{}
	observer := &recordingObserver{}
	reader, err := NewCommitmentReader("payer", api, 2, observer, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reader.ReadReservations(context.Background(), time.Now())
	if err != nil || !result.HasUtilization || !result.HasCoverage {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if api.utilizationCalls != 2 || api.coverageCalls != 2 {
		t.Fatalf("calls utilization=%d coverage=%d", api.utilizationCalls, api.coverageCalls)
	}
	if got := observer.pageOps; len(got) != 4 || got[0] != common.OperationGetReservationUtilization || got[1] != common.OperationGetReservationUtilization || got[2] != common.OperationGetReservationCoverage || got[3] != common.OperationGetReservationCoverage {
		t.Fatalf("page operations=%v", got)
	}
	limited, err := NewCommitmentReader("payer", &pagedCommitmentStub{}, 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := limited.ReadReservations(context.Background(), time.Now()); err == nil {
		t.Fatal("accepted reservation pagination beyond limit")
	}
}

type commitmentStub struct{}

func (commitmentStub) GetSavingsPlansUtilization(context.Context, *awscostexplorer.GetSavingsPlansUtilizationInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetSavingsPlansUtilizationOutput, error) {
	return &awscostexplorer.GetSavingsPlansUtilizationOutput{Total: &cetypes.SavingsPlansUtilizationAggregates{Utilization: &cetypes.SavingsPlansUtilization{UtilizationPercentage: aws.String("80"), UnusedCommitment: aws.String("2")}, Savings: &cetypes.SavingsPlansSavings{NetSavings: aws.String("4")}}}, nil
}
func (commitmentStub) GetSavingsPlansCoverage(context.Context, *awscostexplorer.GetSavingsPlansCoverageInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetSavingsPlansCoverageOutput, error) {
	return &awscostexplorer.GetSavingsPlansCoverageOutput{SavingsPlansCoverages: []cetypes.SavingsPlansCoverage{{Coverage: &cetypes.SavingsPlansCoverageData{CoveragePercentage: aws.String("75"), SpendCoveredBySavingsPlans: aws.String("75"), OnDemandCost: aws.String("25"), TotalCost: aws.String("100")}}}}, nil
}
func (commitmentStub) GetReservationUtilization(context.Context, *awscostexplorer.GetReservationUtilizationInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetReservationUtilizationOutput, error) {
	return &awscostexplorer.GetReservationUtilizationOutput{Total: &cetypes.ReservationAggregates{UtilizationPercentage: aws.String("50"), UnusedHours: aws.String("3"), NetRISavings: aws.String("5")}}, nil
}
func (commitmentStub) GetReservationCoverage(context.Context, *awscostexplorer.GetReservationCoverageInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetReservationCoverageOutput, error) {
	return &awscostexplorer.GetReservationCoverageOutput{Total: &cetypes.Coverage{CoverageHours: &cetypes.CoverageHours{CoverageHoursPercentage: aws.String("60")}, CoverageCost: &cetypes.CoverageCost{OnDemandCost: aws.String("40")}}}, nil
}

type pagedCommitmentStub struct {
	utilizationCalls int
	coverageCalls    int
}

func (stub *pagedCommitmentStub) GetSavingsPlansUtilization(context.Context, *awscostexplorer.GetSavingsPlansUtilizationInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetSavingsPlansUtilizationOutput, error) {
	return (&commitmentStub{}).GetSavingsPlansUtilization(context.Background(), nil)
}
func (stub *pagedCommitmentStub) GetSavingsPlansCoverage(context.Context, *awscostexplorer.GetSavingsPlansCoverageInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetSavingsPlansCoverageOutput, error) {
	return (&commitmentStub{}).GetSavingsPlansCoverage(context.Background(), nil)
}
func (stub *pagedCommitmentStub) GetReservationUtilization(_ context.Context, input *awscostexplorer.GetReservationUtilizationInput, _ ...func(*awscostexplorer.Options)) (*awscostexplorer.GetReservationUtilizationOutput, error) {
	stub.utilizationCalls++
	if input.NextPageToken == nil {
		return &awscostexplorer.GetReservationUtilizationOutput{Total: &cetypes.ReservationAggregates{UtilizationPercentage: aws.String("50")}, NextPageToken: aws.String("next")}, nil
	}
	return &awscostexplorer.GetReservationUtilizationOutput{Total: &cetypes.ReservationAggregates{UtilizationPercentage: aws.String("50")}}, nil
}
func (stub *pagedCommitmentStub) GetReservationCoverage(_ context.Context, input *awscostexplorer.GetReservationCoverageInput, _ ...func(*awscostexplorer.Options)) (*awscostexplorer.GetReservationCoverageOutput, error) {
	stub.coverageCalls++
	if input.NextPageToken == nil {
		return &awscostexplorer.GetReservationCoverageOutput{Total: &cetypes.Coverage{CoverageHours: &cetypes.CoverageHours{CoverageHoursPercentage: aws.String("60")}}, NextPageToken: aws.String("next")}, nil
	}
	return &awscostexplorer.GetReservationCoverageOutput{Total: &cetypes.Coverage{CoverageHours: &cetypes.CoverageHours{CoverageHoursPercentage: aws.String("60")}}}, nil
}

func TestAnomalyReaderAggregatesWithoutIdentifiers(t *testing.T) {
	api := &anomalyStub{}
	reader, err := NewAnomalyReader("payer", api, 2, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reader.Read(context.Background(), time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 2 || !result.Active || result.Impact.Amount() != 7 || result.MaxImpact.Amount() != 5 || result.LastDetected.Format(time.DateOnly) != "2026-07-21" {
		t.Fatalf("summary=%#v", result)
	}
	if value, err := NewAnomalyReader("payer", nil, 1, nil, nil); value != nil || err == nil {
		t.Fatal("accepted nil API")
	}
}

type anomalyStub struct{ calls int }

func (stub *anomalyStub) GetAnomalies(context.Context, *awscostexplorer.GetAnomaliesInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetAnomaliesOutput, error) {
	stub.calls++
	if stub.calls == 1 {
		return &awscostexplorer.GetAnomaliesOutput{Anomalies: []cetypes.Anomaly{{AnomalyEndDate: aws.String("2026-07-20"), Impact: &cetypes.Impact{TotalImpact: 2, MaxImpact: 2}}}, NextPageToken: aws.String("next")}, nil
	}
	return &awscostexplorer.GetAnomaliesOutput{Anomalies: []cetypes.Anomaly{{AnomalyEndDate: aws.String("2026-07-21"), Impact: &cetypes.Impact{TotalImpact: 5, MaxImpact: 5}}}}, nil
}
