// Package athena implements the fixed-schema CUR 2.0 query provider.
package athena

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsathena "github.com/aws/aws-sdk-go-v2/service/athena"
	athenatypes "github.com/aws/aws-sdk-go-v2/service/athena/types"

	"github.com/sakuya1998/aws-cost-exporter/internal/aws/common"
	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/tagcost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

type API interface {
	StartQueryExecution(context.Context, *awsathena.StartQueryExecutionInput, ...func(*awsathena.Options)) (*awsathena.StartQueryExecutionOutput, error)
	GetQueryExecution(context.Context, *awsathena.GetQueryExecutionInput, ...func(*awsathena.Options)) (*awsathena.GetQueryExecutionOutput, error)
	GetQueryResults(context.Context, *awsathena.GetQueryResultsInput, ...func(*awsathena.Options)) (*awsathena.GetQueryResultsOutput, error)
}

type Reader struct {
	target   identity.TargetID
	api      API
	config   config.TargetCURConfig
	maxPages int
	maxRows  int
	observer common.Observer
	retryer  func(string) aws.Retryer
}

func NewReader(target identity.TargetID, api API, value config.TargetCURConfig, maxPages, maxRows int, observer common.Observer, retryer func(string) aws.Retryer) (*Reader, error) {
	if api == nil {
		return nil, fmt.Errorf("athena API must not be nil")
	}
	if maxPages <= 0 || maxRows <= 0 {
		return nil, fmt.Errorf("athena bounds must be positive")
	}
	if observer == nil {
		observer = common.DiscardObserver{}
	}
	return &Reader{target: target, api: api, config: value, maxPages: maxPages, maxRows: maxRows, observer: observer, retryer: retryer}, nil
}

func (reader *Reader) ReadCosts(ctx context.Context, reference time.Time, bases []cost.Basis) ([]cost.Cost, error) {
	rows, err := reader.query(ctx, reference, bases, false)
	if err != nil {
		return nil, err
	}
	return mapCosts(reader.target, reference, rows)
}
func (reader *Reader) QueryCosts(ctx context.Context, reference time.Time, bases []cost.Basis) ([]cost.Cost, error) {
	return reader.ReadCosts(ctx, reference, bases)
}

func (reader *Reader) ReadTagCosts(ctx context.Context, reference time.Time, bases []cost.Basis) ([]tagcost.Cost, error) {
	rows, err := reader.query(ctx, reference, bases, true)
	if err != nil {
		return nil, err
	}
	return mapTagCosts(reader.target, reference, rows)
}
func (reader *Reader) QueryTagCosts(ctx context.Context, reference time.Time, bases []cost.Basis) ([]tagcost.Cost, error) {
	return reader.ReadTagCosts(ctx, reference, bases)
}

func (reader *Reader) query(ctx context.Context, reference time.Time, bases []cost.Basis, tags bool) ([][]string, error) {
	query := buildQuery(reader.config, reference, bases, tags)
	started := time.Now()
	start, err := reader.api.StartQueryExecution(ctx, &awsathena.StartQueryExecutionInput{
		QueryString: aws.String(query), WorkGroup: aws.String(reader.config.Workgroup),
		QueryExecutionContext: &athenatypes.QueryExecutionContext{Database: aws.String(reader.config.Database)},
		ResultConfiguration:   &athenatypes.ResultConfiguration{OutputLocation: aws.String(reader.config.OutputLocation)},
	}, func(options *awsathena.Options) {
		if reader.retryer != nil {
			options.Retryer = reader.retryer(common.OperationStartQueryExecution)
		}
	})
	common.ObserveCall(reader.observer, reader.target, common.OperationStartQueryExecution, started, err)
	if err != nil {
		return nil, common.ClassifyError(err)
	}
	if start == nil || start.QueryExecutionId == nil || *start.QueryExecutionId == "" {
		return nil, fmt.Errorf("athena query returned no execution id")
	}
	id := *start.QueryExecutionId
	deadline := time.Now().Add(reader.config.QueryTimeout)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, context.DeadlineExceeded
		}
		started = time.Now()
		status, callErr := reader.api.GetQueryExecution(ctx, &awsathena.GetQueryExecutionInput{QueryExecutionId: aws.String(id)}, func(options *awsathena.Options) {
			if reader.retryer != nil {
				options.Retryer = reader.retryer(common.OperationGetQueryExecution)
			}
		})
		common.ObserveCall(reader.observer, reader.target, common.OperationGetQueryExecution, started, callErr)
		if callErr != nil {
			return nil, common.ClassifyError(callErr)
		}
		if status == nil || status.QueryExecution == nil || status.QueryExecution.Status == nil {
			return nil, fmt.Errorf("athena query returned no status")
		}
		execution := QueryExecution{ID: id, State: mapQueryState(status.QueryExecution.Status.State)}
		switch execution.State {
		case QuerySucceeded:
			return reader.results(ctx, id)
		case QueryFailed, QueryCancelled:
			return nil, fmt.Errorf("athena query did not succeed: %s", execution.State)
		}
		timer := time.NewTimer(reader.config.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (reader *Reader) results(ctx context.Context, id string) ([][]string, error) {
	var rows [][]string
	var token *string
	for page := 0; page < reader.maxPages; page++ {
		started := time.Now()
		output, err := reader.api.GetQueryResults(ctx, &awsathena.GetQueryResultsInput{QueryExecutionId: aws.String(id), NextToken: token, MaxResults: aws.Int32(1000)}, func(options *awsathena.Options) {
			if reader.retryer != nil {
				options.Retryer = reader.retryer(common.OperationGetQueryResults)
			}
		})
		common.ObserveCall(reader.observer, reader.target, common.OperationGetQueryResults, started, err)
		if err != nil {
			return nil, common.ClassifyError(err)
		}
		if output == nil || output.ResultSet == nil {
			return nil, fmt.Errorf("athena query returned no result set")
		}
		reader.observer.ObservePaginationPage(reader.target, common.OperationGetQueryResults)
		for _, row := range output.ResultSet.Rows {
			values := make([]string, len(row.Data))
			for index, value := range row.Data {
				values[index] = aws.ToString(value.VarCharValue)
			}
			rows = append(rows, values)
			if len(rows) > reader.maxRows {
				return nil, fmt.Errorf("athena result row limit exceeded")
			}
		}
		if output.NextToken == nil || *output.NextToken == "" {
			return rows, nil
		}
		token = output.NextToken
	}
	return nil, fmt.Errorf("athena result page limit exceeded")
}

func buildQuery(value config.TargetCURConfig, reference time.Time, bases []cost.Basis, tags bool) string {
	day := cost.DayContaining(reference)
	month := cost.MonthContaining(reference)
	windows := []struct{ name, start, end string }{
		{string(cost.WindowDaily), day.Start().Format(time.DateOnly), day.End().Format(time.DateOnly)},
		{string(cost.WindowMonthToDate), month.Start().Format(time.DateOnly), day.End().Format(time.DateOnly)},
	}
	table := quoteIdentifier(value.Database) + "." + quoteIdentifier(value.Table)
	parts := make([]string, 0)
	for _, basis := range bases {
		expression, ok := basisExpression(basis)
		if !ok {
			continue
		}
		for _, window := range windows {
			if tags {
				for _, tag := range value.TagColumns {
					parts = append(parts, fmt.Sprintf("SELECT '%s' AS window, line_item_currency_code AS currency, '%s' AS cost_basis, SUM(%s) AS amount, '%s' AS tag_key, COALESCE(NULLIF(%s, ''), '__untagged__') AS tag_value FROM %s WHERE line_item_usage_start_date >= DATE '%s' AND line_item_usage_start_date < DATE '%s' GROUP BY 2,6", window.name, basis, expression, strings.ReplaceAll(tag.Key, "'", "''"), quoteIdentifier(tag.Column), table, window.start, window.end))
				}
				continue
			}
			parts = append(parts, fmt.Sprintf("SELECT '%s' AS window, line_item_currency_code AS currency, '%s' AS cost_basis, SUM(%s) AS amount FROM %s WHERE line_item_usage_start_date >= DATE '%s' AND line_item_usage_start_date < DATE '%s' GROUP BY 2", window.name, basis, expression, table, window.start, window.end))
		}
	}
	return strings.Join(parts, " UNION ALL ") + " ORDER BY window, currency, cost_basis"
}

func basisExpression(value cost.Basis) (string, bool) {
	switch value {
	case cost.BasisUnblended:
		return "COALESCE(line_item_unblended_cost, 0)", true
	case cost.BasisAmortized:
		return "COALESCE(savings_plan_savings_plan_effective_cost, reservation_effective_cost, line_item_unblended_cost, 0)", true
	case cost.BasisNet:
		return "COALESCE(line_item_net_unblended_cost, line_item_unblended_cost, 0)", true
	default:
		return "", false
	}
}

func quoteIdentifier(value string) string { return `"` + strings.ReplaceAll(value, `"`, `""`) + `"` }

func mapCosts(target identity.TargetID, reference time.Time, rows [][]string) ([]cost.Cost, error) {
	if len(rows) > 0 && strings.EqualFold(rows[0][0], "window") {
		rows = rows[1:]
	}
	day, month := cost.DayContaining(reference), cost.MonthContaining(reference)
	values := make([]cost.Cost, 0, len(rows))
	for _, row := range rows {
		if len(row) < 4 {
			return nil, fmt.Errorf("athena cost row has fewer than four columns")
		}
		amount, err := strconv.ParseFloat(row[3], 64)
		if err != nil {
			return nil, err
		}
		money, err := cost.NewMoney(amount, row[1])
		if err != nil {
			return nil, err
		}
		basis := cost.Basis(row[2])
		if !basis.Valid() {
			return nil, fmt.Errorf("unsupported CUR cost basis")
		}
		window := cost.Window(row[0])
		period := day
		if window == cost.WindowMonthToDate {
			period = month
		} else if window != cost.WindowDaily {
			return nil, fmt.Errorf("unsupported CUR window")
		}
		values = append(values, cost.Cost{Target: target, Provider: cost.ProviderCURAthena, Basis: basis, Window: window, Period: period, Dimension: mustTotalDimension(), Amount: money})
	}
	return values, nil
}

func mapTagCosts(target identity.TargetID, reference time.Time, rows [][]string) ([]tagcost.Cost, error) {
	if len(rows) > 0 && strings.EqualFold(rows[0][0], "window") {
		rows = rows[1:]
	}
	_ = reference
	values := make([]tagcost.Cost, 0, len(rows))
	for _, row := range rows {
		if len(row) < 6 {
			return nil, fmt.Errorf("athena tag row has fewer than six columns")
		}
		amount, err := strconv.ParseFloat(row[3], 64)
		if err != nil {
			return nil, err
		}
		money, err := cost.NewMoney(amount, row[1])
		if err != nil {
			return nil, err
		}
		basis := cost.Basis(row[2])
		if !basis.Valid() {
			return nil, fmt.Errorf("unsupported CUR cost basis")
		}
		window := cost.Window(row[0])
		if window != cost.WindowDaily && window != cost.WindowMonthToDate {
			return nil, fmt.Errorf("unsupported CUR window")
		}
		values = append(values, tagcost.Cost{Target: target, Provider: cost.ProviderCURAthena, Basis: basis, Window: window, TagKey: row[4], TagValue: row[5], Amount: money})
	}
	return values, nil
}

func mustTotalDimension() cost.Dimension {
	value, _ := cost.NewDimension(cost.DimensionTotal, "")
	return value
}

var _ ports.CURReader = (*Reader)(nil)
