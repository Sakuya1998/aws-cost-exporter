package costexplorer

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

// TestMapUsageMapsTotalAndGroupedCosts verifies decimal, period, currency, and
// dimension normalization without leaking AWS SDK types into the domain.
func TestMapUsageMapsTotalAndGroupedCosts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, wantValue string
		groupBy         cost.DimensionKind
		result          cetypes.ResultByTime
	}{
		{name: "total", groupBy: cost.DimensionTotal, result: usageResult("12.50")},
		{name: "global region", groupBy: cost.DimensionRegion, wantValue: "global", result: usageResult("12.50", "")},
	}
	for _, test := range tests {
		mapped, err := MapUsage([]cetypes.ResultByTime{test.result}, ports.CostQuery{
			Window: cost.WindowDaily, GroupBy: test.groupBy,
		}, metricUnblendedCost)
		if err != nil {
			t.Fatalf("%s: MapUsage() returned an unexpected error: %v", test.name, err)
		}
		if len(mapped) != 1 || mapped[0].Amount.Amount() != 12.5 ||
			mapped[0].Amount.Currency() != "USD" ||
			mapped[0].Dimension.Value() != test.wantValue ||
			mapped[0].Period.Start().Format("2006-01-02") != "2026-07-01" {
			t.Fatalf("%s: MapUsage() = %#v, want one normalized USD cost", test.name, mapped)
		}
	}
}

func TestMapUsageAggregatesMTDTotalAcrossDays(t *testing.T) {
	t.Parallel()
	period, err := cost.NewPeriod(
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("NewPeriod() error = %v", err)
	}
	results := []cetypes.ResultByTime{
		usageResultOnDay("2026-07-01", "2026-07-02", "10"),
		usageResultOnDay("2026-07-02", "2026-07-03", "20"),
		usageResultOnDay("2026-07-03", "2026-07-04", "30"),
	}
	mapped, err := MapUsage(results, ports.CostQuery{
		Period: period, Window: cost.WindowMonthToDate, GroupBy: cost.DimensionTotal,
	}, metricUnblendedCost)
	if err != nil {
		t.Fatalf("MapUsage() error = %v", err)
	}
	if len(mapped) != 1 {
		t.Fatalf("MapUsage() len = %d, want 1 aggregated MTD cost", len(mapped))
	}
	if mapped[0].Amount.Amount() != 60 || mapped[0].Amount.Currency() != "USD" ||
		mapped[0].Window != cost.WindowMonthToDate ||
		!mapped[0].Period.Start().Equal(period.Start()) ||
		!mapped[0].Period.End().Equal(period.End()) {
		t.Fatalf("MapUsage() = %#v, want 60 USD over full MTD period", mapped[0])
	}
}

func TestMapUsageAggregatesMTDGroupedAcrossDays(t *testing.T) {
	t.Parallel()
	period, err := cost.NewPeriod(
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("NewPeriod() error = %v", err)
	}
	results := []cetypes.ResultByTime{
		usageResultOnDay("2026-07-01", "2026-07-02", "1", "Amazon EC2"),
		usageResultOnDay("2026-07-02", "2026-07-03", "2", "Amazon EC2"),
		usageResultOnDay("2026-07-03", "2026-07-04", "3", "Amazon EC2"),
	}
	mapped, err := MapUsage(results, ports.CostQuery{
		Period: period, Window: cost.WindowMonthToDate, GroupBy: cost.DimensionService,
	}, metricUnblendedCost)
	if err != nil {
		t.Fatalf("MapUsage() error = %v", err)
	}
	if len(mapped) != 1 || mapped[0].Dimension.Value() != "Amazon EC2" ||
		mapped[0].Amount.Amount() != 6 {
		t.Fatalf("MapUsage() = %#v, want one EC2 MTD cost of 6 USD", mapped)
	}
}

func TestMapUsageDailyDoesNotAggregate(t *testing.T) {
	t.Parallel()
	period, err := cost.NewPeriod(
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("NewPeriod() error = %v", err)
	}
	results := []cetypes.ResultByTime{
		usageResultOnDay("2026-07-01", "2026-07-02", "10"),
		usageResultOnDay("2026-07-02", "2026-07-03", "20"),
		usageResultOnDay("2026-07-03", "2026-07-04", "30"),
	}
	mapped, err := MapUsage(results, ports.CostQuery{
		Period: period, Window: cost.WindowDaily, GroupBy: cost.DimensionTotal,
	}, metricUnblendedCost)
	if err != nil {
		t.Fatalf("MapUsage() error = %v", err)
	}
	if len(mapped) != 3 {
		t.Fatalf("MapUsage() len = %d, want 3 daily costs", len(mapped))
	}
}

func TestMapUsageMTDKeepsMixedCurrencySeparate(t *testing.T) {
	t.Parallel()
	period, err := cost.NewPeriod(
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("NewPeriod() error = %v", err)
	}
	results := []cetypes.ResultByTime{
		usageResultOnDay("2026-07-01", "2026-07-02", "10", "Amazon EC2"),
		usageResultWithCurrency("2026-07-01", "2026-07-02", "5", "EUR", "Amazon EC2"),
	}
	mapped, err := MapUsage(results, ports.CostQuery{
		Period: period, Window: cost.WindowMonthToDate, GroupBy: cost.DimensionService,
	}, metricUnblendedCost)
	if err != nil {
		t.Fatalf("MapUsage() error = %v", err)
	}
	if len(mapped) != 2 {
		t.Fatalf("MapUsage() len = %d, want separate USD and EUR series", len(mapped))
	}
}

// TestMapUsageRejectsInvalidResults verifies malformed groups and amounts
// produce no partially mapped costs.
func TestMapUsageRejectsInvalidResults(t *testing.T) {
	tests := []struct {
		name    string
		result  cetypes.ResultByTime
		groupBy cost.DimensionKind
		wantErr error
	}{
		{name: "malformed group", result: usageResult("12.50", "one", "two"), groupBy: cost.DimensionService, wantErr: ErrInvalidResponse},
		{name: "non-finite amount", result: usageResult("NaN"), groupBy: cost.DimensionTotal, wantErr: cost.ErrInvalidAmount},
	}
	for _, test := range tests {
		mapped, err := MapUsage([]cetypes.ResultByTime{test.result}, ports.CostQuery{
			Window: cost.WindowDaily, GroupBy: test.groupBy,
		}, metricUnblendedCost)
		if len(mapped) != 0 || !errors.Is(err, test.wantErr) {
			t.Fatalf("%s: MapUsage() = %#v, %v; want no costs and %v", test.name, mapped, err, test.wantErr)
		}
	}
}

// usageResult constructs a compact Cost Explorer result fixture.
func usageResult(amount string, keys ...string) cetypes.ResultByTime {
	return usageResultOnDay("2026-07-01", "2026-07-02", amount, keys...)
}

func usageResultOnDay(start, end, amount string, keys ...string) cetypes.ResultByTime {
	return usageResultWithCurrency(start, end, amount, "USD", keys...)
}

func usageResultWithCurrency(start, end, amount, currency string, keys ...string) cetypes.ResultByTime {
	metric := map[string]cetypes.MetricValue{"UnblendedCost": {
		Amount: aws.String(amount), Unit: aws.String(currency),
	}}
	result := cetypes.ResultByTime{TimePeriod: &cetypes.DateInterval{
		Start: aws.String(start), End: aws.String(end),
	}}
	if len(keys) == 0 {
		result.Total = metric
	} else {
		result.Groups = []cetypes.Group{{Keys: keys, Metrics: metric}}
	}
	return result
}
