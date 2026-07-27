package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/anomaly"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/budget"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/commitment"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/organization"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/tagcost"
)

func TestCostCollectorGoldenIncludesTargetAndNewDomains(t *testing.T) {
	subject, err := NewCostCollector(staticStore{snapshot: businessSnapshot(t)})
	if err != nil {
		t.Fatal(err)
	}
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(subject)
	const expected = `
# HELP aws_budget_actual_amount AWS Budget calculated actual spend.
# TYPE aws_budget_actual_amount gauge
aws_budget_actual_amount{budget_name="Monthly",budget_type="COST",currency="USD",target="payer-prod",time_unit="MONTHLY"} 45
# HELP aws_budget_limit_amount Configured AWS Budget limit.
# TYPE aws_budget_limit_amount gauge
aws_budget_limit_amount{budget_name="Monthly",budget_type="COST",currency="USD",target="payer-prod",time_unit="MONTHLY"} 100
# HELP aws_cost_account_info Non-sensitive AWS Organizations metadata for an exported linked account.
# TYPE aws_cost_account_info gauge
aws_cost_account_info{account_name="production",account_status="ACTIVE",linked_account_id="123456789012",target="payer-prod"} 1
# HELP aws_cost_daily_amount Current UTC billing day accumulated cost.
# TYPE aws_cost_daily_amount gauge
aws_cost_daily_amount{cost_basis="unblended",currency="USD",provider="cost_explorer",target="payer-prod"} 1
# HELP aws_cost_month_forecast_mean_amount Forecast mean for the remaining current UTC month, including today.
# TYPE aws_cost_month_forecast_mean_amount gauge
aws_cost_month_forecast_mean_amount{cost_basis="unblended",currency="USD",provider="cost_explorer",target="payer-prod"} 100
# HELP aws_cost_service_daily_amount Current UTC billing day cost by AWS service.
# TYPE aws_cost_service_daily_amount gauge
aws_cost_service_daily_amount{aws_service="Amazon EC2",cost_basis="unblended",currency="USD",provider="cost_explorer",target="payer-prod"} 3
`
	if err := testutil.GatherAndCompare(registry, strings.NewReader(expected), "aws_budget_actual_amount", "aws_budget_limit_amount", "aws_cost_account_info", "aws_cost_daily_amount", "aws_cost_month_forecast_mean_amount", "aws_cost_service_daily_amount"); err != nil {
		t.Fatal(err)
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		for _, metric := range family.Metric {
			found := false
			for _, label := range metric.Label {
				if label.GetName() == "target" && label.GetValue() == "payer-prod" {
					found = true
				}
			}
			if !found {
				t.Fatalf("metric %s lacks target label", family.GetName())
			}
		}
	}
}

func TestBudgetMissingForecastOmitsSeries(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	subject, _ := NewCostCollector(staticStore{snapshot: businessSnapshot(t)})
	registry.MustRegister(subject)
	families, _ := registry.Gather()
	for _, family := range families {
		if family.GetName() == "aws_budget_forecasted_amount" {
			t.Fatal("missing forecast emitted a zero series")
		}
	}
}

func TestV03MetricsKeepProviderBasisAndBoundedSummaryLabels(t *testing.T) {
	target := identity.TargetID("payer-prod")
	money, _ := cost.NewMoney(12, "USD")
	value := snapshot.NewWithData(nil, nil, nil, nil,
		[]commitment.Summary{{Target: target, Type: commitment.TypeSavingsPlan, TimeUnit: "MONTHLY", UtilizationRatio: .8, HasUtilization: true, NetSavings: money, HasNetSavings: true}},
		[]anomaly.Summary{{Target: target, Active: true, Count: 2, Impact: money, HasImpact: true}},
		[]tagcost.Cost{{Target: target, Provider: cost.ProviderCURAthena, Basis: cost.BasisNet, Window: cost.WindowDaily, TagKey: "Environment", TagValue: "prod", Amount: money}},
	)
	collector, _ := NewCostCollector(staticStore{snapshot: value})
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(collector)
	text, err := testutil.GatherAndCount(registry, "aws_commitment_utilization_ratio", "aws_commitment_net_savings_amount", "aws_cost_anomaly_count", "aws_cost_tag_daily_amount")
	if err != nil || text != 4 {
		t.Fatalf("v0.3 metric families=%d err=%v", text, err)
	}
	families, _ := registry.Gather()
	serialized := ""
	for _, family := range families {
		serialized += family.String()
	}
	for _, fragment := range []string{`cur_athena`, `net`, `savings_plan`} {
		if !strings.Contains(serialized, fragment) {
			t.Fatalf("metrics lack %s", fragment)
		}
	}
	for _, forbidden := range []string{"anomaly_id", "root_cause", "plan_id"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("metrics expose high-cardinality label %s", forbidden)
		}
	}
}

func TestNewCostCollectorRejectsNilAndIgnoresUnknownCost(t *testing.T) {
	if subject, err := NewCostCollector(nil); subject != nil || err == nil {
		t.Fatal("accepted nil store")
	}
	money, _ := cost.NewMoney(1, "USD")
	subject, _ := NewCostCollector(staticStore{snapshot: snapshot.New([]cost.Cost{{Target: "payer-prod", Amount: money}}, nil, nil, nil)})
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(subject)
	families, err := registry.Gather()
	if err != nil || len(families) != 0 {
		t.Fatalf("unknown cost gather=%v,%v", families, err)
	}
}

type staticStore struct{ snapshot snapshot.Snapshot }

func (value staticStore) Snapshot() snapshot.Snapshot { return value.snapshot }

func businessSnapshot(t *testing.T) snapshot.Snapshot {
	t.Helper()
	target := identity.TargetID("payer-prod")
	day := cost.DayContaining(time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC))
	month := cost.MonthContaining(day.Start())
	values := []struct {
		window    cost.Window
		kind      cost.DimensionKind
		dimension string
		amount    float64
	}{{cost.WindowDaily, cost.DimensionTotal, "", 1}, {cost.WindowMonthToDate, cost.DimensionTotal, "", 2}, {cost.WindowDaily, cost.DimensionService, "Amazon EC2", 3}, {cost.WindowMonthToDate, cost.DimensionService, "Amazon EC2", 4}, {cost.WindowDaily, cost.DimensionRegion, "us-east-1", 5}, {cost.WindowMonthToDate, cost.DimensionRegion, "us-east-1", 6}, {cost.WindowDaily, cost.DimensionAccount, "123456789012", 7}, {cost.WindowMonthToDate, cost.DimensionAccount, "123456789012", 8}}
	var costs []cost.Cost
	for _, value := range values {
		dimension, _ := cost.NewDimension(value.kind, value.dimension)
		money, _ := cost.NewMoney(value.amount, "USD")
		period := day
		if value.window == cost.WindowMonthToDate {
			period = month
		}
		costs = append(costs, cost.Cost{Target: target, Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Window: value.window, Period: period, Dimension: dimension, Amount: money})
	}
	mean, _ := cost.NewMoney(100, "USD")
	lower, _ := cost.NewMoney(90, "USD")
	upper, _ := cost.NewMoney(110, "USD")
	limit, _ := cost.NewMoney(100, "USD")
	actual, _ := cost.NewMoney(45, "USD")
	return snapshot.New(costs, []cost.Forecast{{Target: target, Provider: cost.ProviderCostExplorer, Basis: cost.BasisUnblended, Period: month, Mean: mean, LowerBound: lower, UpperBound: upper}}, []budget.Budget{{Target: target, Name: "Monthly", Type: "COST", TimeUnit: "MONTHLY", Limit: limit, Actual: actual, HasActual: true}}, []organization.Account{{Target: target, AccountID: "123456789012", Name: "production", Status: "ACTIVE"}})
}
