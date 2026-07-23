package costexplorer

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	"github.com/sakuya1998/aws-cost-exporter/internal/aws/common"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/anomaly"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

type AnomalyAPI interface {
	GetAnomalies(context.Context, *awscostexplorer.GetAnomaliesInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetAnomaliesOutput, error)
}

type AnomalyReader struct {
	target   identity.TargetID
	api      AnomalyAPI
	maxPages int
	observer common.Observer
	retryer  func(string) aws.Retryer
}

func NewAnomalyReader(target identity.TargetID, api AnomalyAPI, maxPages int, observer common.Observer, retryer func(string) aws.Retryer) (*AnomalyReader, error) {
	if api == nil {
		return nil, fmt.Errorf("anomaly API must not be nil")
	}
	if maxPages <= 0 {
		return nil, fmt.Errorf("anomaly max pages must be positive")
	}
	if observer == nil {
		observer = common.DiscardObserver{}
	}
	return &AnomalyReader{target: target, api: api, maxPages: maxPages, observer: observer, retryer: retryer}, nil
}

func (reader *AnomalyReader) ReadAnomalySummary(ctx context.Context, reference time.Time) (anomaly.Summary, error) {
	start := reference.UTC().AddDate(0, 0, -90).Format(time.DateOnly)
	end := reference.UTC().Format(time.DateOnly)
	input := &awscostexplorer.GetAnomaliesInput{DateInterval: &cetypes.AnomalyDateInterval{StartDate: aws.String(start), EndDate: aws.String(end)}, MaxResults: aws.Int32(100)}
	result := anomaly.Summary{Target: reader.target}
	for page := 0; page < reader.maxPages; page++ {
		started := time.Now()
		output, err := reader.api.GetAnomalies(ctx, input, func(options *awscostexplorer.Options) {
			if reader.retryer != nil {
				options.Retryer = reader.retryer(common.OperationGetAnomalies)
			}
		})
		common.ObserveCall(reader.observer, reader.target, common.OperationGetAnomalies, started, err)
		if err != nil {
			return anomaly.Summary{}, ClassifyError(err)
		}
		if output == nil {
			return anomaly.Summary{}, fmt.Errorf("invalid anomaly response")
		}
		reader.observer.ObservePaginationPage(reader.target, common.OperationGetAnomalies)
		for _, item := range output.Anomalies {
			result.Count++
			if anomalyEndIsActive(item, reference) {
				result.Active = true
			}
			if item.Impact != nil {
				value, parseErr := cost.NewMoney(item.Impact.TotalImpact, "USD")
				if parseErr != nil {
					return anomaly.Summary{}, parseErr
				}
				if !result.HasImpact {
					result.Impact, result.HasImpact = value, true
				} else if sum, sumErr := result.Impact.Add(value); sumErr == nil {
					result.Impact = sum
				}
				maxValue, maxErr := cost.NewMoney(item.Impact.MaxImpact, "USD")
				if maxErr == nil && (!result.HasMaxImpact || maxValue.Amount() > result.MaxImpact.Amount()) {
					result.MaxImpact, result.HasMaxImpact = maxValue, true
				}
			}
			if item.AnomalyEndDate != nil {
				if instant, parseErr := time.Parse(time.DateOnly, *item.AnomalyEndDate); parseErr == nil && instant.After(result.LastDetected) {
					result.LastDetected = instant
				}
			}
		}
		if output.NextPageToken == nil || *output.NextPageToken == "" {
			break
		}
		input.NextPageToken = output.NextPageToken
		if page == reader.maxPages-1 {
			return anomaly.Summary{}, fmt.Errorf("anomaly page limit exceeded")
		}
	}
	return result, nil
}

func anomalyEndIsActive(item cetypes.Anomaly, reference time.Time) bool {
	if item.AnomalyEndDate == nil {
		return true
	}
	date, err := time.Parse(time.DateOnly, *item.AnomalyEndDate)
	return err != nil || !date.Before(reference.UTC().Truncate(24*time.Hour))
}

var _ ports.AnomalyReader = (*AnomalyReader)(nil)
