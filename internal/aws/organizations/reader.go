// Package organizations adapts AWS Organizations to bounded domain metadata.
package organizations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/organizations"

	awscommon "github.com/sakuya1998/aws-cost-exporter/internal/aws/common"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	domain "github.com/sakuya1998/aws-cost-exporter/internal/domain/organization"
)

var (
	ErrInvalidConfig   = errors.New("invalid Organizations adapter configuration")
	ErrInvalidResponse = errors.New("invalid Organizations response")
	ErrPageLimit       = errors.New("organizations page limit exceeded")
	ErrSeriesLimit     = errors.New("organizations account limit exceeded")
)

// Policy bounds and optionally allowlists metadata retained from Organizations.
type Policy struct {
	AccountIDs  []string
	SeriesLimit int
}

type API interface {
	ListAccounts(context.Context, *organizations.ListAccountsInput, ...func(*organizations.Options)) (*organizations.ListAccountsOutput, error)
	DescribeOrganization(context.Context, *organizations.DescribeOrganizationInput, ...func(*organizations.Options)) (*organizations.DescribeOrganizationOutput, error)
}

// Reader validates organization context and retrieves all account metadata.
type Reader struct {
	target      identity.TargetID
	api         API
	maxPages    int
	allowlist   map[string]struct{}
	seriesLimit int
	observer    awscommon.Observer
	retryer     func(string) aws.Retryer
}

func New(target identity.TargetID, api API, maxPages int, policy Policy, observer awscommon.Observer, retryer func(string) aws.Retryer) (*Reader, error) {
	if target == "" || api == nil || maxPages <= 0 || policy.SeriesLimit <= 0 || len(policy.AccountIDs) > policy.SeriesLimit || retryer == nil {
		return nil, ErrInvalidConfig
	}
	if observer == nil {
		observer = awscommon.DiscardObserver{}
	}
	allowlist := make(map[string]struct{}, len(policy.AccountIDs))
	for _, id := range policy.AccountIDs {
		if !accountID(id) {
			return nil, ErrInvalidConfig
		}
		if _, duplicate := allowlist[id]; duplicate {
			return nil, ErrInvalidConfig
		}
		allowlist[id] = struct{}{}
	}
	return &Reader{
		target: target, api: api, maxPages: maxPages, allowlist: allowlist,
		seriesLimit: policy.SeriesLimit, observer: observer, retryer: retryer,
	}, nil
}

// ReadAccounts returns an all-or-nothing, email-free metadata result.
func (reader *Reader) ReadAccounts(ctx context.Context) ([]domain.Account, error) {
	started := time.Now()
	described, err := reader.api.DescribeOrganization(ctx, &organizations.DescribeOrganizationInput{}, func(options *organizations.Options) {
		options.Retryer = reader.retryer(awscommon.OperationDescribeOrganization)
	})
	awscommon.ObserveCall(reader.observer, reader.target, awscommon.OperationDescribeOrganization, started, err)
	if err != nil {
		return nil, awscommon.ClassifyError(err)
	}
	if described == nil || described.Organization == nil {
		return nil, fmt.Errorf("%w: missing organization", ErrInvalidResponse)
	}

	request := &organizations.ListAccountsInput{}
	seenTokens, seenAccounts := make(map[string]struct{}), make(map[string]struct{})
	var values []domain.Account
	for page := 1; ; page++ {
		if page > reader.maxPages {
			return nil, ErrPageLimit
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		started = time.Now()
		output, callErr := reader.api.ListAccounts(ctx, request, func(options *organizations.Options) {
			options.Retryer = reader.retryer(awscommon.OperationListAccounts)
		})
		awscommon.ObserveCall(reader.observer, reader.target, awscommon.OperationListAccounts, started, callErr)
		if callErr != nil {
			return nil, awscommon.ClassifyError(callErr)
		}
		if output == nil {
			return nil, fmt.Errorf("%w: nil page", ErrInvalidResponse)
		}
		reader.observer.ObservePaginationPage(reader.target, awscommon.OperationListAccounts)
		for _, account := range output.Accounts {
			id, name := aws.ToString(account.Id), aws.ToString(account.Name)
			if !accountID(id) || name == "" {
				return nil, fmt.Errorf("%w: malformed account", ErrInvalidResponse)
			}
			if _, duplicate := seenAccounts[id]; duplicate {
				return nil, fmt.Errorf("%w: duplicate account", ErrInvalidResponse)
			}
			seenAccounts[id] = struct{}{}
			if len(reader.allowlist) != 0 {
				if _, selected := reader.allowlist[id]; !selected {
					continue
				}
			}
			if len(values) >= reader.seriesLimit {
				return nil, ErrSeriesLimit
			}
			values = append(values, domain.Account{Target: reader.target, AccountID: id, Name: name, Status: accountStatus(string(account.State), string(account.Status))})
		}
		token := aws.ToString(output.NextToken)
		if token == "" {
			domain.Sort(values)
			return values, nil
		}
		if _, duplicate := seenTokens[token]; duplicate {
			return nil, fmt.Errorf("%w: duplicate page token", ErrInvalidResponse)
		}
		seenTokens[token] = struct{}{}
		request.NextToken = aws.String(token)
	}
}

func accountStatus(state, status string) string {
	if state != "" {
		switch state {
		case "PENDING_ACTIVATION", "ACTIVE", "SUSPENDED", "PENDING_CLOSURE", "CLOSED":
			return state
		}
	}
	switch status {
	case "ACTIVE", "SUSPENDED", "PENDING_CLOSURE":
		return status
	}
	return "UNKNOWN"
}

func accountID(value string) bool {
	if len(value) != 12 {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}
