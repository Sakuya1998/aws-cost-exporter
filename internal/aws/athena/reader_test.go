package athena

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awsathena "github.com/aws/aws-sdk-go-v2/service/athena"
	athenatypes "github.com/aws/aws-sdk-go-v2/service/athena/types"

	"github.com/sakuya1998/aws-cost-exporter/internal/aws/common"
	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

func TestReaderRunsFixedQueryAndMapsProviderBasis(t *testing.T) {
	api := &stubAPI{state: athenatypes.QueryExecutionStateSucceeded, rows: [][]string{{"window", "currency", "cost_basis", "amount"}, {"daily", "USD", "net", "2.5"}, {"month_to_date", "USD", "net", "10"}}}
	value := config.TargetCURConfig{Database: "billing", Table: "cur2", Workgroup: "exporter", OutputLocation: "s3://results/", QueryTimeout: time.Second, PollInterval: time.Millisecond}
	reader, err := NewReader("payer", api, value, 2, 10, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reader.QueryCosts(context.Background(), time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC), []cost.Basis{cost.BasisNet})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 || result[0].Provider != cost.ProviderCURAthena || result[0].Basis != cost.BasisNet {
		t.Fatalf("result=%#v", result)
	}
	if strings.Contains(api.query, ";") || !strings.Contains(api.query, `"billing"."cur2"`) || !strings.Contains(api.query, "line_item_net_unblended_cost") {
		t.Fatalf("unsafe or incomplete query: %s", api.query)
	}
}

func TestReaderRejectsFailedAndOverLimitQueries(t *testing.T) {
	value := config.TargetCURConfig{Database: "billing", Table: "cur2", Workgroup: "exporter", OutputLocation: "s3://results/", QueryTimeout: time.Second, PollInterval: time.Millisecond}
	failed, _ := NewReader("payer", &stubAPI{state: athenatypes.QueryExecutionStateFailed}, value, 2, 10, nil, nil)
	if _, err := failed.QueryCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended}); err == nil {
		t.Fatal("accepted failed query")
	}
	over := &stubAPI{state: athenatypes.QueryExecutionStateSucceeded, rows: [][]string{{"window", "currency", "cost_basis", "amount"}, {"daily", "USD", "unblended", "1"}, {"month_to_date", "USD", "unblended", "2"}}}
	limited, _ := NewReader("payer", over, value, 2, 1, nil, nil)
	if _, err := limited.QueryCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended}); err == nil {
		t.Fatal("accepted over-limit result")
	}
}

func TestReaderMapsTagRowsAndStopsOnCancellation(t *testing.T) {
	value := config.TargetCURConfig{Database: "billing", Table: "cur2", Workgroup: "exporter", OutputLocation: "s3://results/", QueryTimeout: time.Second, PollInterval: time.Millisecond, TagColumns: []config.CURTagColumn{{Key: "Environment", Column: "resource_tags_user_environment"}}}
	api := &stubAPI{state: athenatypes.QueryExecutionStateSucceeded, rows: [][]string{{"window", "currency", "cost_basis", "amount", "tag_key", "tag_value"}, {"daily", "USD", "amortized", "3", "Environment", "prod"}}}
	reader, _ := NewReader("payer", api, value, 2, 10, nil, nil)
	tags, err := reader.QueryTagCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisAmortized})
	if err != nil || len(tags) != 1 || tags[0].TagValue != "prod" {
		t.Fatalf("tags=%#v err=%v", tags, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	running, _ := NewReader("payer", &stubAPI{state: athenatypes.QueryExecutionStateRunning}, value, 2, 10, nil, nil)
	if _, err := running.QueryCosts(ctx, time.Now(), []cost.Basis{cost.BasisUnblended}); err == nil {
		t.Fatal("ignored cancellation")
	}
	if running.api.(*stubAPI).stops != 1 {
		t.Fatal("canceled query was not stopped")
	}
	for _, state := range []athenatypes.QueryExecutionState{athenatypes.QueryExecutionStateCancelled} {
		failed, _ := NewReader("payer", &stubAPI{state: state}, value, 2, 10, nil, nil)
		if _, err := failed.QueryCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended}); err == nil {
			t.Fatal("accepted canceled query")
		}
	}
}

func TestReaderValidatesDependenciesAndMalformedResponses(t *testing.T) {
	value := config.TargetCURConfig{Database: "billing", Table: "cur2", Workgroup: "exporter", OutputLocation: "s3://results/", QueryTimeout: time.Second, PollInterval: time.Millisecond}
	if reader, err := NewReader("payer", nil, value, 1, 1, nil, nil); reader != nil || err == nil {
		t.Fatal("accepted nil Athena API")
	}
	if reader, err := NewReader("payer", &stubAPI{}, value, 0, 1, nil, nil); reader != nil || err == nil {
		t.Fatal("accepted invalid page bound")
	}
	if reader, err := NewReader("payer", &stubAPI{}, value, 1, 0, nil, nil); reader != nil || err == nil {
		t.Fatal("accepted invalid row bound")
	}
	invalidTiming := value
	invalidTiming.PollInterval = invalidTiming.QueryTimeout
	if reader, err := NewReader("payer", &stubAPI{}, invalidTiming, 1, 1, nil, nil); reader != nil || err == nil {
		t.Fatal("accepted invalid Athena query timing")
	}
	for _, api := range []*stubAPI{{startErr: errors.New("start")}, {emptyID: true}, {emptyResults: true}} {
		reader, err := NewReader("payer", api, value, 1, 10, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reader.QueryCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended}); err == nil {
			t.Fatalf("accepted malformed response from %#v", api)
		}
	}
	for _, api := range []*stubAPI{{emptyStatus: true}, {statusErr: errors.New("status")}, {state: athenatypes.QueryExecutionState("unknown")}} {
		reader, err := NewReader("payer", api, value, 1, 10, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reader.QueryCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended}); err == nil || api.stops != 1 {
			t.Fatalf("nonterminal failure error=%v stops=%d", err, api.stops)
		}
	}
	timeoutValue := value
	timeoutValue.QueryTimeout = 5 * time.Millisecond
	timeoutValue.PollInterval = time.Millisecond
	timeoutReader, _ := NewReader("payer", &stubAPI{state: athenatypes.QueryExecutionStateRunning}, timeoutValue, 1, 10, nil, nil)
	if _, err := timeoutReader.QueryCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error=%v", err)
	}
	if timeoutReader.api.(*stubAPI).stops != 1 {
		t.Fatal("timed out query was not stopped")
	}
}

func TestReaderTimeoutCoversStartQueryExecution(t *testing.T) {
	value := config.TargetCURConfig{Database: "billing", Table: "cur2", Workgroup: "exporter", OutputLocation: "s3://results/", QueryTimeout: 10 * time.Millisecond, PollInterval: time.Millisecond}
	reader, _ := NewReader("payer", &stubAPI{blockStart: true}, value, 1, 10, nil, nil)
	_, err := reader.QueryCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended})
	var classified *common.ClassifiedError
	if !errors.As(err, &classified) || classified.Kind() != common.ErrorTimeout || !classified.Retryable() {
		t.Fatalf("start timeout error=%v", err)
	}
}

func TestReaderAppliesDualLimiterToEverySDKAttempt(t *testing.T) {
	var statusCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/x-amz-json-1.1")
		switch {
		case strings.HasSuffix(request.Header.Get("X-Amz-Target"), ".StartQueryExecution"):
			_, _ = fmt.Fprint(writer, `{"QueryExecutionId":"query-1"}`)
		case strings.HasSuffix(request.Header.Get("X-Amz-Target"), ".GetQueryExecution"):
			statusCalls++
			state := "RUNNING"
			if statusCalls > 1 {
				state = "SUCCEEDED"
			}
			_, _ = fmt.Fprintf(writer, `{"QueryExecution":{"Status":{"State":%q}}}`, state)
		case strings.HasSuffix(request.Header.Get("X-Amz-Target"), ".GetQueryResults"):
			_, _ = fmt.Fprint(writer, `{"ResultSet":{"ResultSetMetadata":{"ColumnInfo":[{"Name":"window","Type":"varchar"},{"Name":"currency","Type":"varchar"},{"Name":"cost_basis","Type":"varchar"},{"Name":"amount","Type":"decimal"}]},"Rows":[{"Data":[{"VarCharValue":"window"},{"VarCharValue":"currency"},{"VarCharValue":"cost_basis"},{"VarCharValue":"amount"}]},{"Data":[{"VarCharValue":"daily"},{"VarCharValue":"USD"},{"VarCharValue":"unblended"},{"VarCharValue":"1"}]}]}}`)
		default:
			http.Error(writer, "unexpected Athena operation", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	global, target := &attemptCounter{}, &attemptCounter{}
	sdkConfig := aws.Config{
		Region: "us-east-1", HTTPClient: server.Client(),
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("access", "secret", "")),
		Retryer:     func() aws.Retryer { return aws.NopRetryer{} },
	}
	client := awsathena.NewFromConfig(sdkConfig, func(options *awsathena.Options) { options.BaseEndpoint = aws.String(server.URL) })
	retryer := func(operation string) aws.Retryer {
		return common.WrapRetryer(aws.NopRetryer{}, "payer", operation, common.DualLimiter{Global: global, Target: target}, nil)
	}
	value := config.TargetCURConfig{Database: "billing", Table: "cur2", Workgroup: "exporter", OutputLocation: "s3://results/", QueryTimeout: time.Second, PollInterval: time.Millisecond}
	reader, _ := NewReader("payer", client, value, 2, 10, nil, retryer)
	result, err := reader.QueryCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended})
	if err != nil || len(result) != 1 {
		t.Fatalf("SDK endpoint result=%#v err=%v", result, err)
	}
	if global.Calls() != 4 || target.Calls() != 4 {
		t.Fatalf("limiter calls global=%d target=%d, want four each", global.Calls(), target.Calls())
	}
}

func TestReaderPollsStateMachineAndPaginatesStrictResults(t *testing.T) {
	value := config.TargetCURConfig{Database: "billing", Table: "cur2", Workgroup: "exporter", OutputLocation: "s3://results/", QueryTimeout: time.Second, PollInterval: time.Millisecond}
	api := &stubAPI{
		states: []athenatypes.QueryExecutionState{athenatypes.QueryExecutionStateQueued, athenatypes.QueryExecutionStateRunning, athenatypes.QueryExecutionStateSucceeded},
		pages: []stubPage{
			{rows: [][]string{{"window", "currency", "cost_basis", "amount"}, {"daily", "USD", "unblended", "1"}}, nextToken: "page-2"},
			{rows: [][]string{{"month_to_date", "USD", "unblended", "2"}}},
		},
	}
	reader, _ := NewReader("payer", api, value, 3, 2, nil, nil)
	result, err := reader.QueryCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended})
	if err != nil || len(result) != 2 || api.statusCalls != 3 || api.resultCalls != 2 {
		t.Fatalf("result=%#v err=%v status_calls=%d result_calls=%d", result, err, api.statusCalls, api.resultCalls)
	}

	duplicate := &stubAPI{state: athenatypes.QueryExecutionStateSucceeded, pages: []stubPage{
		{rows: [][]string{{"window", "currency", "cost_basis", "amount"}}, nextToken: "same"},
		{rows: nil, nextToken: "same"},
	}}
	reader, _ = NewReader("payer", duplicate, value, 3, 2, nil, nil)
	if _, err := reader.QueryCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended}); err == nil || !strings.Contains(err.Error(), "pagination token") {
		t.Fatalf("duplicate token error=%v", err)
	}
}

func TestReaderRejectsUnexpectedMetadataHeadersAndRows(t *testing.T) {
	value := config.TargetCURConfig{Database: "billing", Table: "cur2", Workgroup: "exporter", OutputLocation: "s3://results/", QueryTimeout: time.Second, PollInterval: time.Millisecond}
	tests := []struct {
		name string
		api  *stubAPI
	}{
		{"missing metadata", &stubAPI{state: athenatypes.QueryExecutionStateSucceeded, omitMetadata: true, rows: [][]string{{"window", "currency", "cost_basis", "amount"}}}},
		{"wrong metadata", &stubAPI{state: athenatypes.QueryExecutionStateSucceeded, metadata: resultMetadata([]string{"currency", "window", "cost_basis", "amount"}), rows: [][]string{{"window", "currency", "cost_basis", "amount"}}}},
		{"missing header", &stubAPI{state: athenatypes.QueryExecutionStateSucceeded, rows: nil}},
		{"reordered header", &stubAPI{state: athenatypes.QueryExecutionStateSucceeded, rows: [][]string{{"currency", "window", "cost_basis", "amount"}}}},
		{"extra data column", &stubAPI{state: athenatypes.QueryExecutionStateSucceeded, rows: [][]string{{"window", "currency", "cost_basis", "amount"}, {"daily", "USD", "unblended", "1", "extra"}}}},
		{"repeated header", &stubAPI{state: athenatypes.QueryExecutionStateSucceeded, rows: [][]string{{"window", "currency", "cost_basis", "amount"}, {"window", "currency", "cost_basis", "amount"}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader, _ := NewReader("payer", test.api, value, 2, 10, nil, nil)
			if _, err := reader.QueryCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended}); err == nil {
				t.Fatal("accepted malformed Athena result")
			}
		})
	}
}

func TestAmortizedQueryCoversCommitmentFeeRows(t *testing.T) {
	expression, ok := basisExpression(cost.BasisAmortized)
	if !ok {
		t.Fatal("amortized basis is unsupported")
	}
	for _, required := range []string{"SavingsPlanCoveredUsage", "DiscountedUsage", "SavingsPlanRecurringFee", "SavingsPlanNegation' THEN 0", "SavingsPlanUpfrontFee' THEN 0", "RIFee", "WHEN 'Fee' THEN CASE", "reservation_reservation_a_r_n", "savings_plan_total_commitment_to_date", "reservation_unused_amortized_upfront_fee_for_billing_period", "reservation_unused_recurring_fee"} {
		if !strings.Contains(expression, required) {
			t.Fatalf("amortized expression is missing %q: %s", required, expression)
		}
	}
}

func TestBuildQueryScansCURTableOnce(t *testing.T) {
	value := config.TargetCURConfig{
		Database: "billing", Table: "cur2",
		TagColumns: []config.CURTagColumn{
			{Key: "Environment", Column: "resource_tags_user_environment"},
			{Key: "Team", Column: "resource_tags_user_team"},
		},
	}
	query := buildQuery(value, time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC), []cost.Basis{cost.BasisUnblended, cost.BasisAmortized, cost.BasisNet}, true)
	if count := strings.Count(query, `FROM "billing"."cur2"`); count != 1 {
		t.Fatalf("CUR table references=%d want 1: %s", count, query)
	}
	if strings.Contains(query, "UNION ALL") {
		t.Fatalf("query still repeats table scans with UNION ALL: %s", query)
	}
	for _, fragment := range []string{"CROSS JOIN UNNEST", "Environment", "Team", "daily", "month_to_date", "unblended", "amortized", "net"} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("single-scan query lacks %q: %s", fragment, query)
		}
	}
}

func TestBuildQueryRejectsEmptyExpansionDimensions(t *testing.T) {
	value := config.TargetCURConfig{Database: "billing", Table: "cur2"}
	reference := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	if query := buildQuery(value, reference, nil, false); query != "" {
		t.Fatalf("empty basis query=%q", query)
	}
	if query := buildQuery(value, reference, []cost.Basis{cost.BasisUnblended}, true); query != "" {
		t.Fatalf("empty tag query=%q", query)
	}
}

func TestReaderMapsAndRejectsMalformedRows(t *testing.T) {
	reference := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	if _, err := mapCosts("payer", reference, [][]string{{"daily", "USD"}}); err == nil {
		t.Fatal("accepted short cost row")
	}
	if _, err := mapCosts("payer", reference, [][]string{{"daily", "USD", "bogus", "1"}}); err == nil {
		t.Fatal("accepted invalid cost basis")
	}
	if _, err := mapCosts("payer", reference, [][]string{{"hourly", "USD", "net", "1"}}); err == nil {
		t.Fatal("accepted invalid cost window")
	}
	if _, err := mapTagCosts("payer", reference, [][]string{{"daily", "USD"}}); err == nil {
		t.Fatal("accepted short tag row")
	}
	if _, err := mapTagCosts("payer", reference, [][]string{{"daily", "USD", "net", "x", "Environment", "prod"}}); err == nil {
		t.Fatal("accepted invalid tag amount")
	}
}

func TestQueryStateMappingIsBounded(t *testing.T) {
	values := []struct {
		input athenatypes.QueryExecutionState
		want  QueryState
	}{
		{athenatypes.QueryExecutionStateQueued, QueryQueued},
		{athenatypes.QueryExecutionStateRunning, QueryRunning},
		{athenatypes.QueryExecutionStateSucceeded, QuerySucceeded},
		{athenatypes.QueryExecutionStateFailed, QueryFailed},
		{athenatypes.QueryExecutionStateCancelled, QueryCancelled},
		{athenatypes.QueryExecutionState("unknown"), QueryUnknown},
	}
	for _, value := range values {
		if got := mapQueryState(value.input); got != value.want {
			t.Fatalf("state %q mapped to %q, want %q", value.input, got, value.want)
		}
	}
}

type stubAPI struct {
	state                              athenatypes.QueryExecutionState
	states                             []athenatypes.QueryExecutionState
	rows                               [][]string
	pages                              []stubPage
	metadata                           *athenatypes.ResultSetMetadata
	query                              string
	startErr                           error
	statusErr                          error
	emptyID, emptyStatus, emptyResults bool
	omitMetadata                       bool
	blockStart                         bool
	statusCalls, resultCalls, stops    int
}

type stubPage struct {
	rows      [][]string
	nextToken string
}

func (stub *stubAPI) StartQueryExecution(ctx context.Context, input *awsathena.StartQueryExecutionInput, _ ...func(*awsathena.Options)) (*awsathena.StartQueryExecutionOutput, error) {
	if stub.blockStart {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if stub.startErr != nil {
		return nil, stub.startErr
	}
	stub.query = aws.ToString(input.QueryString)
	if stub.emptyID {
		return &awsathena.StartQueryExecutionOutput{}, nil
	}
	return &awsathena.StartQueryExecutionOutput{QueryExecutionId: aws.String("query-1")}, nil
}
func (stub *stubAPI) GetQueryExecution(context.Context, *awsathena.GetQueryExecutionInput, ...func(*awsathena.Options)) (*awsathena.GetQueryExecutionOutput, error) {
	stub.statusCalls++
	if stub.statusErr != nil {
		return nil, stub.statusErr
	}
	if stub.emptyStatus {
		return &awsathena.GetQueryExecutionOutput{}, nil
	}
	state := stub.state
	if len(stub.states) > 0 {
		index := min(stub.statusCalls-1, len(stub.states)-1)
		state = stub.states[index]
	}
	return &awsathena.GetQueryExecutionOutput{QueryExecution: &athenatypes.QueryExecution{Status: &athenatypes.QueryExecutionStatus{State: state}}}, nil
}
func (stub *stubAPI) GetQueryResults(context.Context, *awsathena.GetQueryResultsInput, ...func(*awsathena.Options)) (*awsathena.GetQueryResultsOutput, error) {
	stub.resultCalls++
	if stub.emptyResults {
		return &awsathena.GetQueryResultsOutput{}, nil
	}
	inputRows := stub.rows
	nextToken := ""
	if len(stub.pages) > 0 {
		index := min(stub.resultCalls-1, len(stub.pages)-1)
		inputRows = stub.pages[index].rows
		nextToken = stub.pages[index].nextToken
	}
	rows := make([]athenatypes.Row, 0, len(inputRows))
	for _, input := range inputRows {
		data := make([]athenatypes.Datum, 0, len(input))
		for _, value := range input {
			data = append(data, athenatypes.Datum{VarCharValue: aws.String(value)})
		}
		rows = append(rows, athenatypes.Row{Data: data})
	}
	metadata := stub.metadata
	if metadata == nil && !stub.omitMetadata {
		columns := costColumns
		if len(inputRows) > 0 && len(inputRows[0]) == len(tagColumns) {
			columns = tagColumns
		}
		metadata = resultMetadata(columns)
	}
	output := &awsathena.GetQueryResultsOutput{ResultSet: &athenatypes.ResultSet{Rows: rows, ResultSetMetadata: metadata}}
	if nextToken != "" {
		output.NextToken = aws.String(nextToken)
	}
	return output, nil
}

func (stub *stubAPI) StopQueryExecution(context.Context, *awsathena.StopQueryExecutionInput, ...func(*awsathena.Options)) (*awsathena.StopQueryExecutionOutput, error) {
	stub.stops++
	return &awsathena.StopQueryExecutionOutput{}, nil
}

func resultMetadata(names []string) *athenatypes.ResultSetMetadata {
	columns := make([]athenatypes.ColumnInfo, len(names))
	for index, name := range names {
		dataType := "varchar"
		if name == "amount" {
			dataType = "decimal"
		}
		columns[index] = athenatypes.ColumnInfo{Name: aws.String(name), Type: aws.String(dataType)}
	}
	return &athenatypes.ResultSetMetadata{ColumnInfo: columns}
}

type attemptCounter struct {
	mu    sync.Mutex
	calls int
}

func (counter *attemptCounter) Wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	counter.mu.Lock()
	counter.calls++
	counter.mu.Unlock()
	return nil
}

func (counter *attemptCounter) Calls() int {
	counter.mu.Lock()
	defer counter.mu.Unlock()
	return counter.calls
}
