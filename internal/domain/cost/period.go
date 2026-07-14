package cost

import (
	"errors"
	"time"
)

// ErrInvalidPeriod indicates a period is empty or runs backwards.
var ErrInvalidPeriod = errors.New("period start must precede end")

// Period is a UTC time range with an inclusive start and exclusive end.
type Period struct {
	start time.Time
	end   time.Time
}

// NewPeriod validates a range and normalizes both boundaries to UTC.
func NewPeriod(start, end time.Time) (Period, error) {
	if !start.Before(end) {
		return Period{}, ErrInvalidPeriod
	}

	return Period{
		start: start.UTC(),
		end:   end.UTC(),
	}, nil
}

// DayContaining returns the UTC billing day containing the reference instant.
func DayContaining(reference time.Time) Period {
	reference = reference.UTC()
	start := time.Date(
		reference.Year(),
		reference.Month(),
		reference.Day(),
		0, 0, 0, 0,
		time.UTC,
	)

	return Period{start: start, end: start.AddDate(0, 0, 1)}
}

// MonthContaining returns the UTC calendar month containing the reference
// instant.
func MonthContaining(reference time.Time) Period {
	reference = reference.UTC()
	start := time.Date(reference.Year(), reference.Month(), 1, 0, 0, 0, 0, time.UTC)

	return Period{start: start, end: start.AddDate(0, 1, 0)}
}

// Start returns the inclusive UTC boundary.
func (period Period) Start() time.Time {
	return period.start
}

// End returns the exclusive UTC boundary.
func (period Period) End() time.Time {
	return period.end
}
