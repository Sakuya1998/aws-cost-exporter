package forecast

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

var _ collector.Collector = (*Collector)(nil)

// TestCollectorBuildsRemainingMonthForecast verifies UTC boundaries and snapshot mapping.
func TestCollectorBuildsRemainingMonthForecast(t *testing.T) {
	reader := &recordingReader{}
	subject, _ := New(reader, 80)
	snapshot, err := subject.Collect(context.Background(), time.Date(
		2026, 7, 13, 1, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60),
	))
	forecasts := snapshot.Forecasts()
	if err != nil || subject.Name() != Name || len(forecasts) != 1 ||
		reader.query.PredictionInterval != 80 ||
		reader.query.Period.Start().Format(time.DateOnly) != "2026-07-12" ||
		reader.query.Period.End().Format(time.DateOnly) != "2026-08-01" ||
		forecasts[0].Mean.Amount() != 100 || forecasts[0].Mean.Currency() != "USD" {
		t.Fatalf("Collect() returned error=%v query=%#v forecasts=%#v", err, reader.query, forecasts)
	}
}

// TestCollectorRejectsInvalidWiringAndReaderFailure verifies isolated failures.
func TestCollectorRejectsInvalidWiringAndReaderFailure(t *testing.T) {
	if subject, err := New(nil, 80); subject != nil || !errors.Is(err, ErrNilReader) {
		t.Fatalf("New(nil, 80) = %#v, %v; want ErrNilReader", subject, err)
	}
	if subject, err := New(&recordingReader{}, 79); subject != nil ||
		!errors.Is(err, ErrInvalidPredictionInterval) {
		t.Fatalf("New(reader, 79) = %#v, %v; want invalid interval", subject, err)
	}
	subject, _ := New(&recordingReader{err: errors.New("forecast unavailable")}, 80)
	snapshot, err := subject.Collect(context.Background(), time.Now())
	if err == nil || len(snapshot.Forecasts()) != 0 {
		t.Fatalf("Collect() returned error=%v forecasts=%#v", err, snapshot.Forecasts())
	}
}

type recordingReader struct {
	query ports.ForecastQuery
	err   error
}

func (reader *recordingReader) ReadForecast(_ context.Context, query ports.ForecastQuery) (cost.Forecast, error) {
	reader.query = query
	if reader.err != nil {
		return cost.Forecast{}, reader.err
	}
	mean, _ := cost.NewMoney(100, "USD")
	lower, _ := cost.NewMoney(80, "USD")
	upper, _ := cost.NewMoney(120, "USD")
	return cost.Forecast{
		Period: query.Period, Mean: mean, LowerBound: lower, UpperBound: upper,
	}, nil
}
