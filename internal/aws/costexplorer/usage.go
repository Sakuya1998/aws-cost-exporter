package costexplorer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

// ErrNilUsageAPI indicates a missing Cost Explorer usage dependency.
var ErrNilUsageAPI = errors.New("cost explorer usage API must not be nil")

// UsageAdapter implements the normalized, all-or-nothing cost reader port.
type UsageAdapter struct {
	paginator  *UsagePaginator
	costMetric string
}

// NewUsageAdapter validates and constructs a usage adapter.
func NewUsageAdapter(api API, maxPages int, costMetric string, observer Observer) (*UsageAdapter, error) {
	if api == nil {
		return nil, ErrNilUsageAPI
	}
	if costMetric == "" {
		return nil, fmt.Errorf("%w: cost metric must not be empty", ErrInvalidResponse)
	}
	paginator, err := NewUsagePaginator(api, maxPages, observer)
	if err != nil {
		return nil, err
	}
	return &UsageAdapter{paginator: paginator, costMetric: costMetric}, nil
}

// ReadCosts serializes a domain query, reads every page, and maps the result.
func (adapter *UsageAdapter) ReadCosts(ctx context.Context, query ports.CostQuery) ([]cost.Cost, error) {
	input := &awscostexplorer.GetCostAndUsageInput{
		TimePeriod: &cetypes.DateInterval{
			Start: aws.String(query.Period.Start().Format(time.DateOnly)),
			End:   aws.String(query.Period.End().Format(time.DateOnly)),
		},
		Granularity: cetypes.GranularityDaily,
		Metrics:     []string{adapter.costMetric},
		Filter:      usageFilter(query),
	}
	if query.GroupBy != cost.DimensionTotal {
		dimension, err := usageDimension(query.GroupBy)
		if err != nil {
			return nil, err
		}
		input.GroupBy = []cetypes.GroupDefinition{{
			Key: aws.String(string(dimension)), Type: cetypes.GroupDefinitionTypeDimension,
		}}
	}
	results, err := adapter.paginator.Read(ctx, input)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return MapUsage(results, query, adapter.costMetric)
}

func usageDimension(kind cost.DimensionKind) (cetypes.Dimension, error) {
	switch kind {
	case cost.DimensionService:
		return cetypes.DimensionService, nil
	case cost.DimensionRegion:
		return cetypes.DimensionRegion, nil
	case cost.DimensionAccount:
		return cetypes.DimensionLinkedAccount, nil
	default:
		return "", fmt.Errorf("%w: unsupported group %q", ErrInvalidResponse, kind)
	}
}

func usageFilter(query ports.CostQuery) *cetypes.Expression {
	return dimensionFilter(query.LinkedAccountIDs, query.Services, query.Regions)
}

func dimensionFilter(linkedAccountIDs, services, regions []string) *cetypes.Expression {
	values := []struct {
		key    cetypes.Dimension
		values []string
	}{
		{cetypes.DimensionLinkedAccount, linkedAccountIDs},
		{cetypes.DimensionService, services},
		{cetypes.DimensionRegion, regions},
	}
	expressions := make([]cetypes.Expression, 0, len(values))
	for _, value := range values {
		if len(value.values) > 0 {
			expressions = append(expressions, cetypes.Expression{Dimensions: &cetypes.DimensionValues{
				Key: value.key, Values: append([]string(nil), value.values...),
			}})
		}
	}
	if len(expressions) == 0 {
		return nil
	}
	if len(expressions) == 1 {
		return &expressions[0]
	}
	return &cetypes.Expression{And: expressions}
}
