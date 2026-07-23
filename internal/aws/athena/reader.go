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
	StopQueryExecution(context.Context, *awsathena.StopQueryExecutionInput, ...func(*awsathena.Options)) (*awsathena.StopQueryExecutionOutput, error)
}

var (
	costColumns = []string{"window", "currency", "cost_basis", "amount"}
	tagColumns  = []string{"window", "currency", "cost_basis", "amount", "tag_key", "tag_value"}
)

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
	if value.QueryTimeout <= 0 || value.PollInterval <= 0 || value.PollInterval >= value.QueryTimeout {
		return nil, fmt.Errorf("athena query timing must be positive and poll interval must be less than timeout")
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
	if query == "" {
		return nil, fmt.Errorf("athena query has no supported cost basis")
	}
	queryCtx, cancel := context.WithTimeout(ctx, reader.config.QueryTimeout)
	defer cancel()
	expectedColumns := costColumns
	if tags {
		expectedColumns = tagColumns
	}
	started := time.Now()
	start, err := reader.api.StartQueryExecution(queryCtx, &awsathena.StartQueryExecutionInput{
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
	for {
		if err := queryCtx.Err(); err != nil {
			return nil, reader.stopPendingQuery(ctx, id, err)
		}
		started = time.Now()
		status, callErr := reader.api.GetQueryExecution(queryCtx, &awsathena.GetQueryExecutionInput{QueryExecutionId: aws.String(id)}, func(options *awsathena.Options) {
			if reader.retryer != nil {
				options.Retryer = reader.retryer(common.OperationGetQueryExecution)
			}
		})
		common.ObserveCall(reader.observer, reader.target, common.OperationGetQueryExecution, started, callErr)
		if callErr != nil {
			if queryCtx.Err() != nil {
				return nil, reader.stopPendingQuery(ctx, id, queryCtx.Err())
			}
			return nil, common.ClassifyError(callErr)
		}
		if status == nil || status.QueryExecution == nil || status.QueryExecution.Status == nil {
			return nil, fmt.Errorf("athena query returned no status")
		}
		execution := QueryExecution{ID: id, State: mapQueryState(status.QueryExecution.Status.State)}
		switch execution.State {
		case QuerySucceeded:
			return reader.results(queryCtx, id, expectedColumns)
		case QueryFailed, QueryCancelled:
			return nil, fmt.Errorf("athena query did not succeed: %s", execution.State)
		}
		timer := time.NewTimer(reader.config.PollInterval)
		select {
		case <-queryCtx.Done():
			timer.Stop()
			return nil, reader.stopPendingQuery(ctx, id, queryCtx.Err())
		case <-timer.C:
		}
	}
}

func (reader *Reader) results(ctx context.Context, id string, expectedColumns []string) ([][]string, error) {
	var rows [][]string
	var token *string
	seenTokens := make(map[string]struct{})
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
		if err := validateResultMetadata(output.ResultSet.ResultSetMetadata, expectedColumns); err != nil {
			return nil, err
		}
		reader.observer.ObservePaginationPage(reader.target, common.OperationGetQueryResults)
		pageRows := output.ResultSet.Rows
		if page == 0 {
			if len(pageRows) == 0 {
				return nil, fmt.Errorf("athena query returned no header row")
			}
			header, err := decodeRow(pageRows[0], len(expectedColumns))
			if err != nil || !equalColumns(header, expectedColumns) {
				return nil, fmt.Errorf("athena query returned an unexpected header")
			}
			pageRows = pageRows[1:]
		}
		for _, row := range pageRows {
			values, err := decodeRow(row, len(expectedColumns))
			if err != nil {
				return nil, err
			}
			if equalColumns(values, expectedColumns) {
				return nil, fmt.Errorf("athena query repeated its header")
			}
			rows = append(rows, values)
			if len(rows) > reader.maxRows {
				return nil, fmt.Errorf("athena result row limit exceeded")
			}
		}
		if output.NextToken == nil || *output.NextToken == "" {
			return rows, nil
		}
		if _, duplicate := seenTokens[*output.NextToken]; duplicate {
			return nil, fmt.Errorf("athena result repeated pagination token")
		}
		seenTokens[*output.NextToken] = struct{}{}
		token = output.NextToken
	}
	return nil, fmt.Errorf("athena result page limit exceeded")
}

func (reader *Reader) stopPendingQuery(parent context.Context, id string, cause error) error {
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	started := time.Now()
	_, err := reader.api.StopQueryExecution(stopCtx, &awsathena.StopQueryExecutionInput{QueryExecutionId: aws.String(id)}, func(options *awsathena.Options) {
		if reader.retryer != nil {
			options.Retryer = reader.retryer(common.OperationStopQueryExecution)
		}
	})
	common.ObserveCall(reader.observer, reader.target, common.OperationStopQueryExecution, started, err)
	return common.ClassifyError(cause)
}

func validateResultMetadata(metadata *athenatypes.ResultSetMetadata, expected []string) error {
	if metadata == nil || len(metadata.ColumnInfo) != len(expected) {
		return fmt.Errorf("athena query returned unexpected result metadata")
	}
	for index, column := range metadata.ColumnInfo {
		if aws.ToString(column.Name) != expected[index] {
			return fmt.Errorf("athena query returned unexpected result column %d", index)
		}
		dataType := strings.ToLower(aws.ToString(column.Type))
		if expected[index] == "amount" {
			switch dataType {
			case "decimal", "double", "float", "real", "bigint", "integer", "smallint", "tinyint":
			default:
				return fmt.Errorf("athena amount column is not numeric")
			}
		} else if dataType != "varchar" {
			return fmt.Errorf("athena result column %d is not varchar", index)
		}
	}
	return nil
}

func decodeRow(row athenatypes.Row, columns int) ([]string, error) {
	if len(row.Data) != columns {
		return nil, fmt.Errorf("athena result row has unexpected column count")
	}
	values := make([]string, columns)
	for index, value := range row.Data {
		values[index] = aws.ToString(value.VarCharValue)
	}
	return values, nil
}

func equalColumns(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
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
	order := " ORDER BY window, currency, cost_basis"
	if tags {
		order += ", tag_key, tag_value"
	}
	return strings.Join(parts, " UNION ALL ") + order
}

func basisExpression(value cost.Basis) (string, bool) {
	switch value {
	case cost.BasisUnblended:
		return "COALESCE(line_item_unblended_cost, 0)", true
	case cost.BasisAmortized:
		return "CASE line_item_line_item_type WHEN 'SavingsPlanCoveredUsage' THEN COALESCE(savings_plan_savings_plan_effective_cost, 0) WHEN 'DiscountedUsage' THEN COALESCE(reservation_effective_cost, 0) WHEN 'SavingsPlanRecurringFee' THEN COALESCE(savings_plan_total_commitment_to_date, 0) - COALESCE(savings_plan_used_commitment, 0) WHEN 'RIFee' THEN COALESCE(reservation_unused_amortized_upfront_fee_for_billing_period, 0) + COALESCE(reservation_unused_recurring_fee, 0) ELSE COALESCE(line_item_unblended_cost, 0) END", true
	case cost.BasisNet:
		return "COALESCE(line_item_net_unblended_cost, line_item_unblended_cost, 0)", true
	default:
		return "", false
	}
}

func quoteIdentifier(value string) string { return `"` + strings.ReplaceAll(value, `"`, `""`) + `"` }

func mapCosts(target identity.TargetID, reference time.Time, rows [][]string) ([]cost.Cost, error) {
	day, month := cost.DayContaining(reference), cost.MonthContaining(reference)
	values := make([]cost.Cost, 0, len(rows))
	for _, row := range rows {
		if len(row) != len(costColumns) {
			return nil, fmt.Errorf("athena cost row has unexpected column count")
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
	_ = reference
	values := make([]tagcost.Cost, 0, len(rows))
	for _, row := range rows {
		if len(row) != len(tagColumns) {
			return nil, fmt.Errorf("athena tag row has unexpected column count")
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
