package costexplorer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
)

// TestUsagePaginatorReadsEveryEndpointPage verifies pagination against the real
// AWS SDK serializer and a local awsJson endpoint.
func TestUsagePaginatorReadsEveryEndpointPage(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if target := request.Header.Get("X-Amz-Target"); !strings.HasSuffix(target, ".GetCostAndUsage") {
			t.Errorf("X-Amz-Target = %q, want GetCostAndUsage", target)
		}
		writer.Header().Set("Content-Type", "application/x-amz-json-1.1")
		if calls == 1 {
			_, _ = fmt.Fprint(writer, `{"ResultsByTime":[{"TimePeriod":{"Start":"2026-07-01","End":"2026-07-02"}}],"NextPageToken":"page-2"}`)
			return
		}
		_, _ = fmt.Fprint(writer, `{"ResultsByTime":[{"TimePeriod":{"Start":"2026-07-02","End":"2026-07-03"}}]}`)
	}))
	defer server.Close()

	value := config.Default().AWS
	value.EndpointURL = server.URL
	value.RequestTimeout = time.Second
	value.Retry.MaxAttempts = 1
	client, err := New(context.Background(), value)
	if err != nil {
		t.Fatalf("New() returned an unexpected error: %v", err)
	}

	results, err := NewUsagePaginator(client, 50, nil).Read(context.Background(), validUsageInput())
	if err != nil {
		t.Fatalf("Read() returned an unexpected error: %v", err)
	}
	if calls != 2 || len(results) != 2 {
		t.Fatalf("Read() made %d calls and returned %d results, want 2 and 2", calls, len(results))
	}
}

// TestUsagePaginatorRejectsIncompletePagination verifies empty pages continue,
// while duplicate tokens and later failures discard earlier results.
func TestUsagePaginatorRejectsIncompletePagination(t *testing.T) {
	pageFailure := errors.New("page failed")
	result := cetypes.ResultByTime{TimePeriod: &cetypes.DateInterval{
		Start: aws.String("2026-07-01"), End: aws.String("2026-07-02"),
	}}
	tests := []struct {
		name    string
		pages   []pageResponse
		wantLen int
		wantErr error
	}{
		{name: "empty page continues", pages: []pageResponse{
			{output: &awscostexplorer.GetCostAndUsageOutput{NextPageToken: aws.String("next")}},
			{output: &awscostexplorer.GetCostAndUsageOutput{ResultsByTime: []cetypes.ResultByTime{result}}},
		}, wantLen: 1},
		{name: "duplicate token", pages: []pageResponse{
			{output: &awscostexplorer.GetCostAndUsageOutput{NextPageToken: aws.String("same")}},
			{output: &awscostexplorer.GetCostAndUsageOutput{NextPageToken: aws.String("same")}},
		}, wantErr: ErrDuplicatePageToken},
		{name: "later failure", pages: []pageResponse{
			{output: &awscostexplorer.GetCostAndUsageOutput{
				ResultsByTime: []cetypes.ResultByTime{result}, NextPageToken: aws.String("next"),
			}},
			{err: pageFailure},
		}, wantErr: pageFailure},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := &pageAPI{pages: test.pages}
			results, err := NewUsagePaginator(api, 50, nil).Read(context.Background(), validUsageInput())
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Read() error = %v, want %v", err, test.wantErr)
			}
			if len(results) != test.wantLen {
				t.Fatalf("Read() returned %d results, want %d", len(results), test.wantLen)
			}
		})
	}
}

func TestUsagePaginatorRejectsPageLimitExceeded(t *testing.T) {
	pages := []pageResponse{
		{output: &awscostexplorer.GetCostAndUsageOutput{NextPageToken: aws.String("page-2")}},
		{output: &awscostexplorer.GetCostAndUsageOutput{NextPageToken: aws.String("page-3")}},
		{output: &awscostexplorer.GetCostAndUsageOutput{}},
	}
	api := &pageAPI{pages: pages}
	_, err := NewUsagePaginator(api, 2, nil).Read(context.Background(), validUsageInput())
	if !errors.Is(err, ErrPageLimitExceeded) {
		t.Fatalf("Read() error = %v, want %v", err, ErrPageLimitExceeded)
	}
}

// pageResponse is one fake paginated API response.
type pageResponse struct {
	output *awscostexplorer.GetCostAndUsageOutput
	err    error
}

// pageAPI returns scripted pages.
type pageAPI struct {
	pages []pageResponse
	next  int
}

// GetCostAndUsage returns the next scripted page.
func (api *pageAPI) GetCostAndUsage(context.Context, *awscostexplorer.GetCostAndUsageInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostAndUsageOutput, error) {
	page := api.pages[api.next]
	api.next++
	return page.output, page.err
}

// GetCostForecast is unused by usage pagination.
func (*pageAPI) GetCostForecast(context.Context, *awscostexplorer.GetCostForecastInput, ...func(*awscostexplorer.Options)) (*awscostexplorer.GetCostForecastOutput, error) {
	return nil, errors.New("unexpected forecast call")
}

// validUsageInput returns a minimal valid SDK request.
func validUsageInput() *awscostexplorer.GetCostAndUsageInput {
	return &awscostexplorer.GetCostAndUsageInput{
		Granularity: cetypes.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
		TimePeriod: &cetypes.DateInterval{
			Start: aws.String("2026-07-01"), End: aws.String("2026-07-03"),
		},
	}
}
