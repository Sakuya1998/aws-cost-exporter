package tagcost

import (
	"testing"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

func TestSort(t *testing.T) {
	values := []Cost{{Target: "z", TagKey: "b"}, {Target: "a", TagKey: "a"}}
	Sort(values)
	if values[0].Target != "a" {
		t.Fatalf("sort=%#v", values)
	}
}

func TestSortUsesBasisWindowKeyAndValue(t *testing.T) {
	euro, _ := cost.NewMoney(1, "EUR")
	dollar, _ := cost.NewMoney(1, "USD")
	values := []Cost{
		{Target: "payer", Provider: "cur_athena", Basis: "amortized", Window: "daily", TagKey: "Environment", TagValue: "prod", Amount: dollar},
		{Target: "payer", Provider: "cost_explorer", Basis: "amortized", Window: "daily", TagKey: "Environment", TagValue: "dev", Amount: dollar},
		{Target: "payer", Provider: "cost_explorer", Basis: "amortized", Window: "daily", TagKey: "Environment", TagValue: "prod", Amount: dollar},
		{Target: "payer", Provider: "cost_explorer", Basis: "amortized", Window: "daily", TagKey: "Environment", TagValue: "prod", Amount: euro},
	}
	Sort(values)
	if values[0].Provider != "cost_explorer" || values[1].Amount.Currency() != "EUR" || values[2].Amount.Currency() != "USD" || values[3].Provider != "cur_athena" {
		t.Fatalf("sorted=%#v", values)
	}
}
