package anomaly

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/sakuya1998/aws-cost-exporter/internal/domain/anomaly"
)

type stubReader struct {
	value domain.Summary
	err   error
}

func (reader stubReader) ReadAnomalySummary(context.Context, time.Time) (domain.Summary, error) {
	return reader.value, reader.err
}

func TestCollectorPublishesAndBoundsSummary(t *testing.T) {
	subject, err := New("payer", stubReader{value: domain.Summary{Target: "payer", Active: true, Count: 2}}, 10)
	if err != nil {
		t.Fatal(err)
	}
	result, err := subject.Collect(context.Background(), time.Now())
	if err != nil || len(result.Anomalies()) != 1 || subject.ID().Name != Name {
		t.Fatalf("result=%#v err=%v", result.Anomalies(), err)
	}
	limited, _ := New("payer", stubReader{value: domain.Summary{Target: "payer", Active: true, Count: 2}}, 1)
	if _, err := limited.Collect(context.Background(), time.Now()); err == nil {
		t.Fatal("accepted summary over series limit")
	}
	failed, _ := New("payer", stubReader{err: errors.New("failed")}, 10)
	if _, err := failed.Collect(context.Background(), time.Now()); err == nil {
		t.Fatal("ignored reader error")
	}
	if value, err := New("payer", nil, 1); value != nil || err == nil {
		t.Fatal("accepted nil reader")
	}
}
