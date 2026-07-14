package costexplorer

import (
	"errors"
	"testing"

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
		})
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
		})
		if len(mapped) != 0 || !errors.Is(err, test.wantErr) {
			t.Fatalf("%s: MapUsage() = %#v, %v; want no costs and %v", test.name, mapped, err, test.wantErr)
		}
	}
}

// usageResult constructs a compact Cost Explorer result fixture.
func usageResult(amount string, keys ...string) cetypes.ResultByTime {
	metric := map[string]cetypes.MetricValue{"UnblendedCost": {
		Amount: aws.String(amount), Unit: aws.String("USD"),
	}}
	result := cetypes.ResultByTime{TimePeriod: &cetypes.DateInterval{
		Start: aws.String("2026-07-01"), End: aws.String("2026-07-02"),
	}}
	if keys == nil {
		result.Total = metric
	} else {
		result.Groups = []cetypes.Group{{Keys: keys, Metrics: metric}}
	}
	return result
}
