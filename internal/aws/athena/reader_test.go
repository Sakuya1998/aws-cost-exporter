package athena

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsathena "github.com/aws/aws-sdk-go-v2/service/athena"
	athenatypes "github.com/aws/aws-sdk-go-v2/service/athena/types"

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
	result, err := reader.ReadCosts(context.Background(), time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC), []cost.Basis{cost.BasisNet})
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
	if _, err := failed.ReadCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended}); err == nil {
		t.Fatal("accepted failed query")
	}
	over := &stubAPI{state: athenatypes.QueryExecutionStateSucceeded, rows: [][]string{{"window", "currency", "cost_basis", "amount"}, {"daily", "USD", "unblended", "1"}}}
	limited, _ := NewReader("payer", over, value, 2, 1, nil, nil)
	if _, err := limited.ReadCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended}); err == nil {
		t.Fatal("accepted over-limit result")
	}
}

func TestReaderMapsTagRowsAndStopsOnCancellation(t *testing.T) {
	value := config.TargetCURConfig{Database: "billing", Table: "cur2", Workgroup: "exporter", OutputLocation: "s3://results/", QueryTimeout: time.Second, PollInterval: time.Millisecond}
	api := &stubAPI{state: athenatypes.QueryExecutionStateSucceeded, rows: [][]string{{"window", "currency", "cost_basis", "amount", "tag_key", "tag_value"}, {"daily", "USD", "amortized", "3", "Environment", "prod"}}}
	reader, _ := NewReader("payer", api, value, 2, 10, nil, nil)
	tags, err := reader.ReadTagCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisAmortized})
	if err != nil || len(tags) != 1 || tags[0].TagValue != "prod" {
		t.Fatalf("tags=%#v err=%v", tags, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	running, _ := NewReader("payer", &stubAPI{state: athenatypes.QueryExecutionStateRunning}, value, 2, 10, nil, nil)
	if _, err := running.ReadCosts(ctx, time.Now(), []cost.Basis{cost.BasisUnblended}); err == nil {
		t.Fatal("ignored cancellation")
	}
	for _, state := range []athenatypes.QueryExecutionState{athenatypes.QueryExecutionStateCancelled} {
		failed, _ := NewReader("payer", &stubAPI{state: state}, value, 2, 10, nil, nil)
		if _, err := failed.ReadCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended}); err == nil {
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
	for _, api := range []*stubAPI{{startErr: errors.New("start")}, {emptyID: true}, {emptyStatus: true}, {emptyResults: true}} {
		reader, err := NewReader("payer", api, value, 1, 10, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reader.ReadCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended}); err == nil {
			t.Fatalf("accepted malformed response from %#v", api)
		}
	}
	timeoutValue := value
	timeoutValue.QueryTimeout = time.Nanosecond
	timeoutReader, _ := NewReader("payer", &stubAPI{state: athenatypes.QueryExecutionStateRunning}, timeoutValue, 1, 10, nil, nil)
	if _, err := timeoutReader.ReadCosts(context.Background(), time.Now(), []cost.Basis{cost.BasisUnblended}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error=%v", err)
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
		{athenatypes.QueryExecutionState("unknown"), QueryFailed},
	}
	for _, value := range values {
		if got := mapQueryState(value.input); got != value.want {
			t.Fatalf("state %q mapped to %q, want %q", value.input, got, value.want)
		}
	}
}

type stubAPI struct {
	state                              athenatypes.QueryExecutionState
	rows                               [][]string
	query                              string
	startErr                           error
	emptyID, emptyStatus, emptyResults bool
}

func (stub *stubAPI) StartQueryExecution(_ context.Context, input *awsathena.StartQueryExecutionInput, _ ...func(*awsathena.Options)) (*awsathena.StartQueryExecutionOutput, error) {
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
	if stub.emptyStatus {
		return &awsathena.GetQueryExecutionOutput{}, nil
	}
	return &awsathena.GetQueryExecutionOutput{QueryExecution: &athenatypes.QueryExecution{Status: &athenatypes.QueryExecutionStatus{State: stub.state}}}, nil
}
func (stub *stubAPI) GetQueryResults(context.Context, *awsathena.GetQueryResultsInput, ...func(*awsathena.Options)) (*awsathena.GetQueryResultsOutput, error) {
	if stub.emptyResults {
		return &awsathena.GetQueryResultsOutput{}, nil
	}
	rows := make([]athenatypes.Row, 0, len(stub.rows))
	for _, input := range stub.rows {
		data := make([]athenatypes.Datum, 0, len(input))
		for _, value := range input {
			data = append(data, athenatypes.Datum{VarCharValue: aws.String(value)})
		}
		rows = append(rows, athenatypes.Row{Data: data})
	}
	return &awsathena.GetQueryResultsOutput{ResultSet: &athenatypes.ResultSet{Rows: rows}}, nil
}
