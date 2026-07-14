package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

// TestCostCollectorGolden verifies all business metric families and labels.
func TestCostCollectorGolden(t *testing.T) {
	subject, err := NewCostCollector(staticStore{snapshot: businessSnapshot(t)})
	if err != nil {
		t.Fatalf("NewCostCollector() error = %v", err)
	}
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(subject)
	const expected = `
# HELP aws_cost_account_daily_amount Current UTC billing day cost by linked account.
# TYPE aws_cost_account_daily_amount gauge
aws_cost_account_daily_amount{currency="USD",linked_account_id="123456789012"} 7
# HELP aws_cost_account_month_to_date_amount Current UTC month-to-date cost by linked account.
# TYPE aws_cost_account_month_to_date_amount gauge
aws_cost_account_month_to_date_amount{currency="USD",linked_account_id="123456789012"} 8
# HELP aws_cost_daily_amount Current UTC billing day accumulated cost.
# TYPE aws_cost_daily_amount gauge
aws_cost_daily_amount{currency="EUR"} 9
aws_cost_daily_amount{currency="USD"} 1
# HELP aws_cost_month_forecast_lower_bound_amount Forecast lower bound for the current UTC month.
# TYPE aws_cost_month_forecast_lower_bound_amount gauge
aws_cost_month_forecast_lower_bound_amount{currency="USD"} 90
# HELP aws_cost_month_forecast_mean_amount Forecast mean for the current UTC month.
# TYPE aws_cost_month_forecast_mean_amount gauge
aws_cost_month_forecast_mean_amount{currency="USD"} 100
# HELP aws_cost_month_forecast_upper_bound_amount Forecast upper bound for the current UTC month.
# TYPE aws_cost_month_forecast_upper_bound_amount gauge
aws_cost_month_forecast_upper_bound_amount{currency="USD"} 110
# HELP aws_cost_month_to_date_amount Current UTC month-to-date accumulated cost.
# TYPE aws_cost_month_to_date_amount gauge
aws_cost_month_to_date_amount{currency="USD"} 2
# HELP aws_cost_region_daily_amount Current UTC billing day cost by AWS region.
# TYPE aws_cost_region_daily_amount gauge
aws_cost_region_daily_amount{aws_region="us-east-1",currency="USD"} 5
# HELP aws_cost_region_month_to_date_amount Current UTC month-to-date cost by AWS region.
# TYPE aws_cost_region_month_to_date_amount gauge
aws_cost_region_month_to_date_amount{aws_region="us-east-1",currency="USD"} 6
# HELP aws_cost_service_daily_amount Current UTC billing day cost by AWS service.
# TYPE aws_cost_service_daily_amount gauge
aws_cost_service_daily_amount{aws_service="Amazon EC2",currency="USD"} 3
# HELP aws_cost_service_month_to_date_amount Current UTC month-to-date cost by AWS service.
# TYPE aws_cost_service_month_to_date_amount gauge
aws_cost_service_month_to_date_amount{aws_service="Amazon EC2",currency="USD"} 4
`
	names := []string{
		"aws_cost_daily_amount", "aws_cost_month_to_date_amount",
		"aws_cost_service_daily_amount", "aws_cost_service_month_to_date_amount",
		"aws_cost_region_daily_amount", "aws_cost_region_month_to_date_amount",
		"aws_cost_account_daily_amount", "aws_cost_account_month_to_date_amount",
		"aws_cost_month_forecast_mean_amount", "aws_cost_month_forecast_lower_bound_amount",
		"aws_cost_month_forecast_upper_bound_amount",
	}
	if err := testutil.GatherAndCompare(registry, strings.NewReader(expected), names...); err != nil {
		t.Fatalf("GatherAndCompare() error = %v", err)
	}
}

// TestNewCostCollectorRejectsNilStore verifies fail-fast dependency validation.
func TestNewCostCollectorRejectsNilStore(t *testing.T) {
	if subject, err := NewCostCollector(nil); subject != nil || err == nil {
		t.Fatalf("NewCostCollector(nil) = %#v, %v; want error", subject, err)
	}
}

// TestCostCollectorIgnoresSnapshotsWithoutKnownMetrics verifies safe absence.
func TestCostCollectorIgnoresSnapshotsWithoutKnownMetrics(t *testing.T) {
	money, _ := cost.NewMoney(1, "USD")
	tests := []cost.Snapshot{
		cost.NewSnapshot(nil, nil),
		cost.NewSnapshot([]cost.Cost{{Amount: money}}, nil),
	}
	for index, snapshot := range tests {
		subject, _ := NewCostCollector(staticStore{snapshot: snapshot})
		registry := prometheus.NewPedanticRegistry()
		registry.MustRegister(subject)
		families, err := registry.Gather()
		if err != nil || len(families) != 0 {
			t.Fatalf("case %d: Gather() = %#v, %v; want no metrics", index, families, err)
		}
	}
}

type staticStore struct{ snapshot cost.Snapshot }

func (store staticStore) Snapshot() cost.Snapshot { return store.snapshot }

func businessSnapshot(t *testing.T) cost.Snapshot {
	t.Helper()
	day := cost.DayContaining(time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC))
	month := cost.MonthContaining(day.Start())
	values := []struct {
		window    cost.Window
		kind      cost.DimensionKind
		dimension string
		amount    float64
	}{
		{cost.WindowDaily, cost.DimensionTotal, "", 1},
		{cost.WindowMonthToDate, cost.DimensionTotal, "", 2},
		{cost.WindowDaily, cost.DimensionService, "Amazon EC2", 3},
		{cost.WindowMonthToDate, cost.DimensionService, "Amazon EC2", 4},
		{cost.WindowDaily, cost.DimensionRegion, "us-east-1", 5},
		{cost.WindowMonthToDate, cost.DimensionRegion, "us-east-1", 6},
		{cost.WindowDaily, cost.DimensionAccount, "123456789012", 7},
		{cost.WindowMonthToDate, cost.DimensionAccount, "123456789012", 8},
	}
	costs := make([]cost.Cost, 0, len(values))
	for _, value := range values {
		dimension, err := cost.NewDimension(value.kind, value.dimension)
		if err != nil {
			t.Fatal(err)
		}
		money, err := cost.NewMoney(value.amount, "USD")
		if err != nil {
			t.Fatal(err)
		}
		period := day
		if value.window == cost.WindowMonthToDate {
			period = month
		}
		costs = append(costs, cost.Cost{
			Window: value.window, Period: period, Dimension: dimension, Amount: money,
		})
	}
	total, _ := cost.NewDimension(cost.DimensionTotal, "")
	euros, _ := cost.NewMoney(9, "EUR")
	costs = append(costs, cost.Cost{
		Window: cost.WindowDaily, Period: day, Dimension: total, Amount: euros,
	})
	mean, _ := cost.NewMoney(100, "USD")
	lower, _ := cost.NewMoney(90, "USD")
	upper, _ := cost.NewMoney(110, "USD")
	return cost.NewSnapshot(costs, []cost.Forecast{{
		Period: month, Mean: mean, LowerBound: lower, UpperBound: upper,
	}})
}
