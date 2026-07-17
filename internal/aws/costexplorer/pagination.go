package costexplorer

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
)

var (
	// ErrDuplicatePageToken indicates Cost Explorer repeated a pagination token.
	ErrDuplicatePageToken = errors.New("duplicate Cost Explorer page token")
	// ErrInvalidPage indicates the SDK returned no page without an error.
	ErrInvalidPage = errors.New("invalid Cost Explorer page")
	// ErrPageLimitExceeded indicates pagination exceeded the configured page budget.
	ErrPageLimitExceeded = errors.New("cost explorer page limit exceeded")
	// ErrInvalidPageLimit indicates a non-positive pagination limit.
	ErrInvalidPageLimit = errors.New("cost explorer page limit must be positive")
)

// UsagePaginator retrieves complete GetCostAndUsage result sets.
type UsagePaginator struct {
	api      API
	maxPages int
	target   identity.TargetID
	observer Observer
}

// NewUsagePaginator constructs an all-or-nothing usage paginator.
func NewUsagePaginator(api API, maxPages int, observer Observer) (*UsagePaginator, error) {
	return NewUsagePaginatorForTarget("default", api, maxPages, observer)
}

// NewUsagePaginatorForTarget constructs a target-scoped all-or-nothing paginator.
func NewUsagePaginatorForTarget(target identity.TargetID, api API, maxPages int, observer Observer) (*UsagePaginator, error) {
	if maxPages <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidPageLimit, maxPages)
	}
	if observer == nil {
		observer = discardObserver{}
	}
	return &UsagePaginator{api: api, maxPages: maxPages, target: target, observer: observer}, nil
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
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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
		paginator.observer.ObservePaginationPage(paginator.target, operationCostAndUsage)

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
