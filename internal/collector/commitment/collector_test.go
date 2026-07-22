package commitment

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/sakuya1998/aws-cost-exporter/internal/domain/commitment"
)

type stubReader struct {
	savings, reservations       domain.Summary
	savingsErr, reservationsErr error
}

func (reader stubReader) ReadSavingsPlans(context.Context, time.Time) (domain.Summary, error) {
	return reader.savings, reader.savingsErr
}
func (reader stubReader) ReadReservations(context.Context, time.Time) (domain.Summary, error) {
	return reader.reservations, reader.reservationsErr
}

func TestCollectorPublishesBothSummariesAndPreservesErrors(t *testing.T) {
	reader := stubReader{savings: domain.Summary{Target: "payer", Type: domain.TypeSavingsPlan, HasUtilization: true}, reservations: domain.Summary{Target: "payer", Type: domain.TypeReservation, HasCoverage: true}}
	subject, err := New("payer", reader, 20)
	if err != nil {
		t.Fatal(err)
	}
	result, err := subject.Collect(context.Background(), time.Now())
	if err != nil || len(result.Commitments()) != 2 || subject.ID().Name != Name {
		t.Fatalf("result=%#v err=%v", result.Commitments(), err)
	}
	reader.savingsErr = errors.New("failed")
	failed, _ := New("payer", reader, 20)
	if _, err := failed.Collect(context.Background(), time.Now()); err == nil {
		t.Fatal("ignored savings error")
	}
	reader.savingsErr, reader.reservationsErr = nil, errors.New("failed")
	failed, _ = New("payer", reader, 20)
	if _, err := failed.Collect(context.Background(), time.Now()); err == nil {
		t.Fatal("ignored reservation error")
	}
	limited, _ := New("payer", stubReader{savings: domain.Summary{Target: "payer", Type: domain.TypeSavingsPlan, HasUtilization: true}, reservations: domain.Summary{Target: "payer", Type: domain.TypeReservation, HasCoverage: true}}, 1)
	if _, err := limited.Collect(context.Background(), time.Now()); err == nil {
		t.Fatal("accepted over-limit summaries")
	}
	if value, err := New("payer", nil, 1); value != nil || err == nil {
		t.Fatal("accepted nil reader")
	}
}
