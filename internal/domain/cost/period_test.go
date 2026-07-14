package cost_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

// TestNewPeriodNormalizesToUTC verifies equivalent instants use one billing
// timezone inside the domain.
func TestNewPeriodNormalizesToUTC(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("UTC+8", 8*60*60)
	start := time.Date(2026, time.July, 10, 8, 0, 0, 0, location)
	end := start.Add(24 * time.Hour)

	period, err := cost.NewPeriod(start, end)
	if err != nil {
		t.Fatalf("NewPeriod() returned an unexpected error: %v", err)
	}

	if got := period.Start(); !got.Equal(start) || got.Location() != time.UTC {
		t.Errorf("Start() = %v in %v, want equivalent UTC instant", got, got.Location())
	}
	if got := period.End(); !got.Equal(end) || got.Location() != time.UTC {
		t.Errorf("End() = %v in %v, want equivalent UTC instant", got, got.Location())
	}
}

// TestNewPeriodRejectsInvalidRange verifies periods are non-empty and forward.
func TestNewPeriodRejectsInvalidRange(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC)
	for _, end := range []time.Time{now, now.Add(-time.Second)} {
		if _, err := cost.NewPeriod(now, end); !errors.Is(err, cost.ErrInvalidPeriod) {
			t.Errorf("NewPeriod(%v, %v) error = %v, want ErrInvalidPeriod", now, end, err)
		}
	}
}

// TestDayContainingUsesUTCBoundaries verifies local dates cannot change AWS
// billing-day boundaries.
func TestDayContainingUsesUTCBoundaries(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("UTC+8", 8*60*60)
	reference := time.Date(2026, time.July, 10, 1, 30, 0, 0, location)
	period := cost.DayContaining(reference)

	wantStart := time.Date(2026, time.July, 9, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC)
	if period.Start() != wantStart || period.End() != wantEnd {
		t.Fatalf("DayContaining() = [%v, %v), want [%v, %v)", period.Start(), period.End(), wantStart, wantEnd)
	}
}

// TestMonthContainingHandlesCalendarBoundaries verifies leap-year and year-end
// month windows use an exclusive first-of-next-month end.
func TestMonthContainingHandlesCalendarBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		reference time.Time
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "leap February",
			reference: time.Date(2024, time.February, 29, 12, 0, 0, 0, time.UTC),
			wantStart: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2024, time.March, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "December rollover",
			reference: time.Date(2026, time.December, 31, 12, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			period := cost.MonthContaining(test.reference)
			if period.Start() != test.wantStart || period.End() != test.wantEnd {
				t.Fatalf("MonthContaining() = [%v, %v), want [%v, %v)", period.Start(), period.End(), test.wantStart, test.wantEnd)
			}
		})
	}
}
