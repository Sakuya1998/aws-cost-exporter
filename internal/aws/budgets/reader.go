// Package budgets adapts allowlisted AWS Budgets to immutable domain values.
package budgets

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	budgettypes "github.com/aws/aws-sdk-go-v2/service/budgets/types"

	awscommon "github.com/sakuya1998/aws-cost-exporter/internal/aws/common"
	domain "github.com/sakuya1998/aws-cost-exporter/internal/domain/budget"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
)

var (
	ErrInvalidConfig   = errors.New("invalid Budgets adapter configuration")
	ErrInvalidResponse = errors.New("invalid Budgets response")
	ErrPageLimit       = errors.New("budgets page limit exceeded")
)

type API interface {
	DescribeBudgets(context.Context, *budgets.DescribeBudgetsInput, ...func(*budgets.Options)) (*budgets.DescribeBudgetsOutput, error)
}

// Reader retrieves only explicitly allowlisted budgets.
type Reader struct {
	target    identity.TargetID
	accountID string
	api       API
	maxPages  int
	allowlist map[string]struct{}
	observer  awscommon.Observer
	retryer   func(string) aws.Retryer
}

func New(target identity.TargetID, accountID string, api API, maxPages int, names []string, observer awscommon.Observer, retryer func(string) aws.Retryer) (*Reader, error) {
	if target == "" || len(accountID) != 12 || api == nil || maxPages <= 0 || len(names) == 0 || retryer == nil {
		return nil, ErrInvalidConfig
	}
	if observer == nil {
		observer = awscommon.DiscardObserver{}
	}
	allowlist := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" {
			return nil, ErrInvalidConfig
		}
		allowlist[name] = struct{}{}
	}
	if len(allowlist) != len(names) {
		return nil, ErrInvalidConfig
	}
	return &Reader{target: target, accountID: accountID, api: api, maxPages: maxPages, allowlist: allowlist, observer: observer, retryer: retryer}, nil
}

// ReadBudgets returns an all-or-nothing allowlisted result.
func (reader *Reader) ReadBudgets(ctx context.Context) ([]domain.Budget, error) {
	request := &budgets.DescribeBudgetsInput{AccountId: aws.String(reader.accountID), MaxResults: aws.Int32(100)}
	seenTokens, found := make(map[string]struct{}), make(map[string]struct{}, len(reader.allowlist))
	var values []domain.Budget
	for page := 1; ; page++ {
		if page > reader.maxPages {
			return nil, ErrPageLimit
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		started := time.Now()
		output, err := reader.api.DescribeBudgets(ctx, request, func(options *budgets.Options) { options.Retryer = reader.retryer(awscommon.OperationDescribeBudgets) })
		awscommon.ObserveCall(reader.observer, reader.target, awscommon.OperationDescribeBudgets, started, err)
		if err != nil {
			return nil, awscommon.ClassifyError(err)
		}
		if output == nil {
			return nil, fmt.Errorf("%w: nil page", ErrInvalidResponse)
		}
		reader.observer.ObservePaginationPage(reader.target, awscommon.OperationDescribeBudgets)
		for _, value := range output.Budgets {
			name := aws.ToString(value.BudgetName)
			if _, selected := reader.allowlist[name]; !selected {
				continue
			}
			if _, duplicate := found[name]; duplicate {
				return nil, fmt.Errorf("%w: duplicate allowlisted budget", ErrInvalidResponse)
			}
			mapped, mapErr := reader.mapBudget(value)
			if mapErr != nil {
				return nil, mapErr
			}
			found[name] = struct{}{}
			values = append(values, mapped)
		}
		token := aws.ToString(output.NextToken)
		if token == "" {
			break
		}
		if _, duplicate := seenTokens[token]; duplicate {
			return nil, fmt.Errorf("%w: duplicate page token", ErrInvalidResponse)
		}
		seenTokens[token] = struct{}{}
		request.NextToken = aws.String(token)
	}
	if len(found) != len(reader.allowlist) {
		return nil, fmt.Errorf("%w: allowlisted budget missing", ErrInvalidResponse)
	}
	domain.Sort(values)
	return values, nil
}

func (reader *Reader) mapBudget(value budgettypes.Budget) (domain.Budget, error) {
	if value.BudgetLimit == nil {
		return domain.Budget{}, fmt.Errorf("%w: budget limit missing", ErrInvalidResponse)
	}
	limit, err := parseSpend(value.BudgetLimit)
	if err != nil {
		return domain.Budget{}, err
	}
	result := domain.Budget{Target: reader.target, Name: aws.ToString(value.BudgetName), Type: string(value.BudgetType), TimeUnit: string(value.TimeUnit), Limit: limit}
	if result.Name == "" || result.Type == "" || result.TimeUnit == "" {
		return domain.Budget{}, fmt.Errorf("%w: required budget metadata missing", ErrInvalidResponse)
	}
	if !knownBudgetType(result.Type) || !knownTimeUnit(result.TimeUnit) {
		return domain.Budget{}, fmt.Errorf("%w: unknown budget enum", ErrInvalidResponse)
	}
	if value.CalculatedSpend != nil {
		if value.CalculatedSpend.ActualSpend != nil {
			actual, spendErr := parseSpend(value.CalculatedSpend.ActualSpend)
			if spendErr != nil {
				return domain.Budget{}, spendErr
			}
			result.Actual, result.HasActual = actual, true
		}
		if value.CalculatedSpend.ForecastedSpend != nil {
			forecasted, spendErr := parseSpend(value.CalculatedSpend.ForecastedSpend)
			if spendErr != nil {
				return domain.Budget{}, spendErr
			}
			result.Forecasted, result.HasForecasted = forecasted, true
		}
	}
	return result, nil
}

func knownBudgetType(value string) bool {
	switch value {
	case "COST", "USAGE", "RI_UTILIZATION", "RI_COVERAGE", "SAVINGS_PLANS_UTILIZATION", "SAVINGS_PLANS_COVERAGE":
		return true
	default:
		return false
	}
}

func knownTimeUnit(value string) bool {
	switch value {
	case "DAILY", "MONTHLY", "QUARTERLY", "ANNUALLY", "CUSTOM":
		return true
	default:
		return false
	}
}

func parseSpend(value *budgettypes.Spend) (cost.Money, error) {
	if value == nil {
		return cost.Money{}, fmt.Errorf("%w: spend missing", ErrInvalidResponse)
	}
	money, err := cost.ParseMoney(aws.ToString(value.Amount), aws.ToString(value.Unit))
	if err != nil {
		return cost.Money{}, fmt.Errorf("%w: invalid spend", ErrInvalidResponse)
	}
	return money, nil
}
