package cost

import "sort"

// Window identifies the billing interval represented by a cost.
type Window string

const (
	// WindowDaily represents one UTC billing day.
	WindowDaily Window = "daily"
	// WindowMonthToDate represents the current UTC calendar month to date.
	WindowMonthToDate Window = "month_to_date"
)

// Cost is one monetary observation for a period and dimension.
type Cost struct {
	Window    Window
	Period    Period
	Dimension Dimension
	Amount    Money
}

// Forecast contains AWS prediction bounds for a future period.
type Forecast struct {
	Period     Period
	Mean       Money
	LowerBound Money
	UpperBound Money
}

// Snapshot is an immutable, deterministically ordered collection result.
type Snapshot struct {
	costs     []Cost
	forecasts []Forecast
}

// PartialSnapshot is the snapshot contract returned by one collector.
type PartialSnapshot = Snapshot

// NewSnapshot copies and orders records before publishing them.
func NewSnapshot(costs []Cost, forecasts []Forecast) Snapshot {
	snapshot := Snapshot{
		costs:     append([]Cost(nil), costs...),
		forecasts: append([]Forecast(nil), forecasts...),
	}

	sort.SliceStable(snapshot.costs, func(left, right int) bool {
		return costLess(snapshot.costs[left], snapshot.costs[right])
	})
	sort.SliceStable(snapshot.forecasts, func(left, right int) bool {
		return snapshot.forecasts[left].Period.Start().Before(
			snapshot.forecasts[right].Period.Start(),
		)
	})

	return snapshot
}

// MergeSnapshots combines collector snapshots without changing their values.
func MergeSnapshots(parts ...PartialSnapshot) Snapshot {
	var costs []Cost
	var forecasts []Forecast
	for _, part := range parts {
		costs = append(costs, part.costs...)
		forecasts = append(forecasts, part.forecasts...)
	}

	return NewSnapshot(costs, forecasts)
}

// Costs returns an isolated copy of the cost records.
func (snapshot Snapshot) Costs() []Cost {
	return append([]Cost(nil), snapshot.costs...)
}

// Forecasts returns an isolated copy of the forecast records.
func (snapshot Snapshot) Forecasts() []Forecast {
	return append([]Forecast(nil), snapshot.forecasts...)
}

// costLess defines deterministic ordering for metric generation.
func costLess(left, right Cost) bool {
	if left.Window != right.Window {
		return left.Window < right.Window
	}
	if left.Dimension.Kind() != right.Dimension.Kind() {
		return left.Dimension.Kind() < right.Dimension.Kind()
	}
	if left.Dimension.Value() != right.Dimension.Value() {
		return left.Dimension.Value() < right.Dimension.Value()
	}

	return left.Period.Start().Before(right.Period.Start())
}
