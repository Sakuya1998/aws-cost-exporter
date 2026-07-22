package athena

import athenatypes "github.com/aws/aws-sdk-go-v2/service/athena/types"

// QueryState is the bounded Athena state used by the polling state machine.
type QueryState string

const (
	QueryQueued    QueryState = "queued"
	QueryRunning   QueryState = "running"
	QuerySucceeded QueryState = "succeeded"
	QueryFailed    QueryState = "failed"
	QueryCancelled QueryState = "cancelled"
)

// QueryExecution contains only safe state required by the application.
type QueryExecution struct {
	ID    string
	State QueryState
}

func mapQueryState(value athenatypes.QueryExecutionState) QueryState {
	switch value {
	case athenatypes.QueryExecutionStateQueued:
		return QueryQueued
	case athenatypes.QueryExecutionStateRunning:
		return QueryRunning
	case athenatypes.QueryExecutionStateSucceeded:
		return QuerySucceeded
	case athenatypes.QueryExecutionStateFailed:
		return QueryFailed
	case athenatypes.QueryExecutionStateCancelled:
		return QueryCancelled
	default:
		return QueryFailed
	}
}
