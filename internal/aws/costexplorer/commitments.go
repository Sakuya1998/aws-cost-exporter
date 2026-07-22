package costexplorer

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	"github.com/sakuya1998/aws-cost-exporter/internal/aws/common"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/commitment"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

type CommitmentAPI interface {
	GetSavingsPlansUtilization(context.Context, *awscostexplorer.GetSavingsPlansUtilizationInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetSavingsPlansUtilizationOutput, error)
	GetSavingsPlansCoverage(context.Context, *awscostexplorer.GetSavingsPlansCoverageInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetSavingsPlansCoverageOutput, error)
	GetReservationUtilization(context.Context, *awscostexplorer.GetReservationUtilizationInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetReservationUtilizationOutput, error)
	GetReservationCoverage(context.Context, *awscostexplorer.GetReservationCoverageInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetReservationCoverageOutput, error)
}

type CommitmentReader struct {
	target   identity.TargetID
	api      CommitmentAPI
	maxPages int
	observer common.Observer
	retryer  func(string) aws.Retryer
}

func NewCommitmentReader(target identity.TargetID, api CommitmentAPI, maxPages int, observer common.Observer, retryer func(string) aws.Retryer) (*CommitmentReader, error) {
	if api == nil {
		return nil, fmt.Errorf("commitment API must not be nil")
	}
	if maxPages <= 0 {
		return nil, fmt.Errorf("commitment max pages must be positive")
	}
	if observer == nil {
		observer = common.DiscardObserver{}
	}
	return &CommitmentReader{target: target, api: api, maxPages: maxPages, observer: observer, retryer: retryer}, nil
}

func (reader *CommitmentReader) Read(ctx context.Context, reference time.Time) (commitment.Summary, error) {
	return reader.ReadSavingsPlans(ctx, reference)
}

func (reader *CommitmentReader) ReadSavingsPlansUtilization(ctx context.Context, reference time.Time) (commitment.Summary, error) {
	result := commitment.Summary{Target: reader.target, TimeUnit: "MONTHLY", Type: commitment.TypeSavingsPlan}
	err := reader.readSavingsPlans(ctx, commitmentPeriod(reference), &result)
	return result, err
}
func (reader *CommitmentReader) ReadSavingsPlansCoverage(ctx context.Context, reference time.Time) (commitment.Summary, error) {
	result := commitment.Summary{Target: reader.target, TimeUnit: "MONTHLY", Type: commitment.TypeSavingsPlan}
	err := reader.readSavingsCoverage(ctx, commitmentPeriod(reference), &result)
	return result, err
}
func (reader *CommitmentReader) ReadReservationUtilization(ctx context.Context, reference time.Time) (commitment.Summary, error) {
	result := commitment.Summary{Target: reader.target, TimeUnit: "MONTHLY", Type: commitment.TypeReservation}
	err := reader.readReservations(ctx, commitmentPeriod(reference), &result)
	return result, err
}
func (reader *CommitmentReader) ReadReservationCoverage(ctx context.Context, reference time.Time) (commitment.Summary, error) {
	result := commitment.Summary{Target: reader.target, TimeUnit: "MONTHLY", Type: commitment.TypeReservation}
	err := reader.readReservationCoverage(ctx, commitmentPeriod(reference), &result)
	return result, err
}

func commitmentPeriod(reference time.Time) *cetypes.DateInterval {
	period := cost.MonthContaining(reference)
	return &cetypes.DateInterval{Start: aws.String(period.Start().Format(time.DateOnly)), End: aws.String(cost.DayContaining(reference).End().Format(time.DateOnly))}
}

func (reader *CommitmentReader) ReadSavingsPlans(ctx context.Context, reference time.Time) (commitment.Summary, error) {
	period := cost.MonthContaining(reference)
	inputPeriod := &cetypes.DateInterval{Start: aws.String(period.Start().Format(time.DateOnly)), End: aws.String(cost.DayContaining(reference).End().Format(time.DateOnly))}
	result := commitment.Summary{Target: reader.target, TimeUnit: "MONTHLY", Type: commitment.TypeSavingsPlan}
	if err := reader.readSavingsPlans(ctx, inputPeriod, &result); err != nil {
		return commitment.Summary{}, err
	}
	if err := reader.readSavingsCoverage(ctx, inputPeriod, &result); err != nil {
		return commitment.Summary{}, err
	}
	return result, nil
}

func (reader *CommitmentReader) ReadReservations(ctx context.Context, reference time.Time) (commitment.Summary, error) {
	period := cost.MonthContaining(reference)
	inputPeriod := &cetypes.DateInterval{Start: aws.String(period.Start().Format(time.DateOnly)), End: aws.String(cost.DayContaining(reference).End().Format(time.DateOnly))}
	result := commitment.Summary{Target: reader.target, TimeUnit: "MONTHLY", Type: commitment.TypeReservation}
	if err := reader.readReservations(ctx, inputPeriod, &result); err != nil {
		return commitment.Summary{}, err
	}
	if err := reader.readReservationCoverage(ctx, inputPeriod, &result); err != nil {
		return commitment.Summary{}, err
	}
	return result, nil
}

func (reader *CommitmentReader) readSavingsCoverage(ctx context.Context, period *cetypes.DateInterval, result *commitment.Summary) error {
	input := &awscostexplorer.GetSavingsPlansCoverageInput{TimePeriod: period, Granularity: cetypes.GranularityMonthly, Metrics: []string{"SpendCoveredBySavingsPlans"}}
	covered, onDemand, total := 0.0, 0.0, 0.0
	for page := 0; page < reader.maxPages; page++ {
		started := time.Now()
		output, err := reader.api.GetSavingsPlansCoverage(ctx, input, func(options *awscostexplorer.Options) {
			if reader.retryer != nil {
				options.Retryer = reader.retryer(common.OperationGetSavingsPlansCoverage)
			}
		})
		common.ObserveCall(reader.observer, reader.target, common.OperationGetSavingsPlansCoverage, started, err)
		if err != nil {
			return ClassifyError(err)
		}
		if output == nil {
			return fmt.Errorf("invalid Savings Plans coverage response")
		}
		reader.observer.ObservePaginationPage(reader.target, common.OperationGetSavingsPlansCoverage)
		for _, item := range output.SavingsPlansCoverages {
			if item.Coverage == nil {
				continue
			}
			var parseErr error
			covered, parseErr = addDecimal(covered, item.Coverage.SpendCoveredBySavingsPlans)
			if parseErr != nil {
				return parseErr
			}
			onDemand, parseErr = addDecimal(onDemand, item.Coverage.OnDemandCost)
			if parseErr != nil {
				return parseErr
			}
			total, parseErr = addDecimal(total, item.Coverage.TotalCost)
			if parseErr != nil {
				return parseErr
			}
		}
		if output.NextToken == nil || *output.NextToken == "" {
			break
		}
		input.NextToken = output.NextToken
		if page == reader.maxPages-1 {
			return fmt.Errorf("savings plans coverage page limit exceeded")
		}
	}
	if total > 0 {
		result.CoverageRatio, result.HasCoverage = covered/total, true
	}
	if money, err := cost.NewMoney(covered, "USD"); err == nil {
		result.CoveredSpend, result.HasCoveredSpend = money, true
	}
	if money, err := cost.NewMoney(onDemand, "USD"); err == nil {
		result.OnDemandCost, result.HasOnDemandCost = money, true
	}
	return nil
}

func (reader *CommitmentReader) readReservationCoverage(ctx context.Context, period *cetypes.DateInterval, result *commitment.Summary) error {
	var token *string
	for page := 0; page < reader.maxPages; page++ {
		started := time.Now()
		output, err := reader.api.GetReservationCoverage(ctx, &awscostexplorer.GetReservationCoverageInput{TimePeriod: period, Granularity: cetypes.GranularityMonthly, Metrics: []string{"Hour", "Cost"}, NextPageToken: token}, func(options *awscostexplorer.Options) {
			if reader.retryer != nil {
				options.Retryer = reader.retryer(common.OperationGetReservationCoverage)
			}
		})
		common.ObserveCall(reader.observer, reader.target, common.OperationGetReservationCoverage, started, err)
		if err != nil {
			return ClassifyError(err)
		}
		if output == nil {
			return fmt.Errorf("invalid Reserved Instance coverage response")
		}
		reader.observer.ObservePaginationPage(reader.target, common.OperationGetReservationCoverage)
		if page == 0 && output.Total != nil {
			if output.Total.CoverageHours != nil && output.Total.CoverageHours.CoverageHoursPercentage != nil {
				value, parseErr := parsePercent(*output.Total.CoverageHours.CoverageHoursPercentage)
				if parseErr != nil {
					return parseErr
				}
				result.CoverageRatio, result.HasCoverage = value, true
			}
			if output.Total.CoverageCost != nil && output.Total.CoverageCost.OnDemandCost != nil {
				money, parseErr := cost.ParseMoney(*output.Total.CoverageCost.OnDemandCost, "USD")
				if parseErr != nil {
					return parseErr
				}
				result.OnDemandCost, result.HasOnDemandCost = money, true
			}
		}
		if output.NextPageToken == nil || *output.NextPageToken == "" {
			return nil
		}
		token = output.NextPageToken
	}
	return fmt.Errorf("reserved instance coverage page limit exceeded")
}

func (reader *CommitmentReader) readSavingsPlans(ctx context.Context, period *cetypes.DateInterval, result *commitment.Summary) error {
	started := time.Now()
	output, err := reader.api.GetSavingsPlansUtilization(ctx, &awscostexplorer.GetSavingsPlansUtilizationInput{TimePeriod: period, Granularity: cetypes.GranularityMonthly}, func(options *awscostexplorer.Options) {
		if reader.retryer != nil {
			options.Retryer = reader.retryer(common.OperationGetSavingsPlansUtilization)
		}
	})
	common.ObserveCall(reader.observer, reader.target, common.OperationGetSavingsPlansUtilization, started, err)
	if err != nil {
		return ClassifyError(err)
	}
	reader.observer.ObservePaginationPage(reader.target, common.OperationGetSavingsPlansUtilization)
	if output == nil || output.Total == nil || output.Total.Utilization == nil {
		return fmt.Errorf("invalid Savings Plans utilization response")
	}
	utilization := output.Total.Utilization
	if utilization.UtilizationPercentage != nil {
		value, parseErr := parsePercent(*utilization.UtilizationPercentage)
		if parseErr != nil {
			return parseErr
		}
		result.UtilizationRatio, result.HasUtilization = value, true
	}
	// Savings Plans expose unused commitment as currency, not hours. The
	// commitment metric contract only publishes unused_hours for reservations.
	if output.Total.Savings != nil && output.Total.Savings.NetSavings != nil {
		money, parseErr := cost.ParseMoney(*output.Total.Savings.NetSavings, "USD")
		if parseErr != nil {
			return parseErr
		}
		result.NetSavings, result.HasNetSavings = money, true
	}
	return nil
}

func (reader *CommitmentReader) readReservations(ctx context.Context, period *cetypes.DateInterval, result *commitment.Summary) error {
	var token *string
	for page := 0; page < reader.maxPages; page++ {
		started := time.Now()
		output, err := reader.api.GetReservationUtilization(ctx, &awscostexplorer.GetReservationUtilizationInput{TimePeriod: period, Granularity: cetypes.GranularityMonthly, NextPageToken: token}, func(options *awscostexplorer.Options) {
			if reader.retryer != nil {
				options.Retryer = reader.retryer(common.OperationGetReservationUtilization)
			}
		})
		common.ObserveCall(reader.observer, reader.target, common.OperationGetReservationUtilization, started, err)
		if err != nil {
			return ClassifyError(err)
		}
		if output == nil {
			return fmt.Errorf("invalid Reserved Instance utilization response")
		}
		reader.observer.ObservePaginationPage(reader.target, common.OperationGetReservationUtilization)
		if page == 0 && output.Total != nil {
			if output.Total.UtilizationPercentage != nil {
				value, parseErr := parsePercent(*output.Total.UtilizationPercentage)
				if parseErr != nil {
					return parseErr
				}
				result.UtilizationRatio, result.HasUtilization = value, true
			}
			if output.Total.UnusedHours != nil {
				value, parseErr := strconv.ParseFloat(*output.Total.UnusedHours, 64)
				if parseErr != nil {
					return parseErr
				}
				result.UnusedHours, result.HasUnusedHours = value, true
			}
			if output.Total.NetRISavings != nil {
				money, parseErr := cost.ParseMoney(*output.Total.NetRISavings, "USD")
				if parseErr != nil {
					return parseErr
				}
				result.NetSavings, result.HasNetSavings = money, true
			}
		}
		if output.NextPageToken == nil || *output.NextPageToken == "" {
			return nil
		}
		token = output.NextPageToken
	}
	return fmt.Errorf("reserved instance utilization page limit exceeded")
}

func parsePercent(value string) (float64, error) {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	if parsed < 0 || parsed > 100 {
		return 0, fmt.Errorf("percentage out of range")
	}
	return parsed / 100, nil
}

func addDecimal(total float64, value *string) (float64, error) {
	if value == nil {
		return total, nil
	}
	parsed, err := strconv.ParseFloat(*value, 64)
	if err != nil {
		return 0, err
	}
	return total + parsed, nil
}

var _ ports.CommitmentReader = (*CommitmentReader)(nil)
