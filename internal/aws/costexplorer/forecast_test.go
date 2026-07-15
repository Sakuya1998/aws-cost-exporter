package costexplorer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

// TestForecastAdapterReadsRealSDKEndpoint verifies request serialization and
// strict mapping through a local awsJson endpoint.
func TestForecastAdapterReadsRealSDKEndpoint(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if !strings.HasSuffix(request.Header.Get("X-Amz-Target"), ".GetCostForecast") ||
			!strings.Contains(string(body), `"PredictionIntervalLevel":80`) ||
			!strings.Contains(string(body), `"Granularity":"MONTHLY"`) || !strings.Contains(string(body), `"Metric":"UNBLENDED_COST"`) ||
			!strings.Contains(string(body), `"Start":"2026-07-01"`) || !strings.Contains(string(body), `"End":"2026-08-01"`) ||
			!strings.Contains(string(body), `"LINKED_ACCOUNT"`) || !strings.Contains(string(body), `"123456789012"`) ||
			!strings.Contains(string(body), `"SERVICE"`) || !strings.Contains(string(body), `"AmazonEC2"`) {
			t.Errorf("unexpected forecast request target=%q body=%s", request.Header.Get("X-Amz-Target"), body)
		}
		writer.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = fmt.Fprint(writer, `{"Total":{"Amount":"100","Unit":"USD"},"ForecastResultsByTime":[{"TimePeriod":{"Start":"2026-07-01","End":"2026-08-01"},"MeanValue":"100","PredictionIntervalLowerBound":"80","PredictionIntervalUpperBound":"120"}]}`)
	}))
	defer server.Close()

	value := config.Default().AWS
	value.EndpointURL, value.RequestTimeout, value.Retry.MaxAttempts = server.URL, time.Second, 1
	client, _ := New(context.Background(), value)
	period, _ := cost.NewPeriod(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	adapter, _ := NewForecastAdapter(client)
	forecast, err := adapter.ReadForecast(context.Background(), ports.ForecastQuery{
		Period: period, PredictionInterval: 80,
		LinkedAccountIDs: []string{"123456789012"}, Services: []string{"AmazonEC2"},
	})
	if err != nil || forecast.Mean.Amount() != 100 ||
		forecast.LowerBound.Amount() != 80 || forecast.UpperBound.Amount() != 120 ||
		forecast.Mean.Currency() != "USD" || forecast.Period.Start() != period.Start() {
		t.Fatalf("ReadForecast() = %#v, %v; want ordered USD forecast", forecast, err)
	}
}

// TestMapForecastRejectsInvalidBounds verifies malformed results cannot enter snapshots.
func TestMapForecastRejectsInvalidBounds(t *testing.T) {
	if adapter, err := NewForecastAdapter(nil); adapter != nil || !errors.Is(err, ErrNilForecastAPI) {
		t.Fatalf("NewForecastAdapter(nil) = %#v, %v; want ErrNilForecastAPI", adapter, err)
	}
	period, _ := cost.NewPeriod(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	output := &awscostexplorer.GetCostForecastOutput{
		Total: &cetypes.MetricValue{Unit: aws.String("USD")},
		ForecastResultsByTime: []cetypes.ForecastResult{{
			TimePeriod: &cetypes.DateInterval{
				Start: aws.String("2026-07-01"), End: aws.String("2026-08-01"),
			},
			MeanValue: aws.String("100"), PredictionIntervalLowerBound: aws.String("120"),
			PredictionIntervalUpperBound: aws.String("80"),
		}},
	}
	for _, invalid := range []*awscostexplorer.GetCostForecastOutput{nil, output} {
		forecast, err := MapForecast(invalid, ports.ForecastQuery{Period: period})
		if !errors.Is(err, ErrInvalidResponse) || forecast != (cost.Forecast{}) {
			t.Fatalf("MapForecast() = %#v, %v; want empty forecast and ErrInvalidResponse", forecast, err)
		}
	}
}
