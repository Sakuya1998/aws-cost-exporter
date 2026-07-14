package costexplorer

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

var (
	// ErrDuplicatePageToken indicates Cost Explorer repeated a pagination token.
	ErrDuplicatePageToken = errors.New("duplicate Cost Explorer page token")
	// ErrInvalidPage indicates the SDK returned no page without an error.
	ErrInvalidPage = errors.New("invalid Cost Explorer page")
	// ErrPageLimitExceeded indicates pagination exceeded the configured page budget.
	ErrPageLimitExceeded = errors.New("Cost Explorer page limit exceeded")
)

// UsagePaginator retrieves complete GetCostAndUsage result sets.
type UsagePaginator struct {
	api      API
	maxPages int
	observer Observer
}

// NewUsagePaginator constructs an all-or-nothing usage paginator.
func NewUsagePaginator(api API, maxPages int, observer Observer) *UsagePaginator {
	if maxPages <= 0 {
		maxPages = 1
	}
	if observer == nil {
		observer = discardObserver{}
	}
	return &UsagePaginator{api: api, maxPages: maxPages, observer: observer}
}

// Read retrieves every page without mutating the caller's input.
func (paginator *UsagePaginator) Read(
	ctx context.Context,
	input *awscostexplorer.GetCostAndUsageInput,
) ([]cetypes.ResultByTime, error) {
	if input == nil {
		return nil, fmt.Errorf("%w: input must not be nil", ErrInvalidPage)
	}

	request := *input
	seen := make(map[string]struct{})
	if token := aws.ToString(request.NextPageToken); token != "" {
		seen[token] = struct{}{}
	}

	var results []cetypes.ResultByTime
	for page := 1; ; page++ {
		if page > paginator.maxPages {
			return nil, fmt.Errorf("%w: limit %d", ErrPageLimitExceeded, paginator.maxPages)
		}
		output, err := paginator.api.GetCostAndUsage(ctx, &request)
		if err != nil {
			return nil, fmt.Errorf("read Cost Explorer page %d: %w", page, err)
		}
		if output == nil {
			return nil, fmt.Errorf("%w: page %d is nil", ErrInvalidPage, page)
		}
		results = append(results, output.ResultsByTime...)
		paginator.observer.ObservePaginationPage(operationCostAndUsage)

		token := aws.ToString(output.NextPageToken)
		if token == "" {
			return results, nil
		}
		if _, duplicate := seen[token]; duplicate {
			return nil, fmt.Errorf("%w at page %d", ErrDuplicatePageToken, page)
		}
		seen[token] = struct{}{}
		request.NextPageToken = aws.String(token)
	}
}
