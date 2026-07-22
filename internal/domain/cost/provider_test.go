package cost_test

import (
	"testing"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

func TestProviderAndBasisAreBounded(t *testing.T) {
	for _, value := range []cost.Provider{cost.ProviderCostExplorer, cost.ProviderCURAthena} {
		if !value.Valid() {
			t.Fatalf("invalid provider %s", value)
		}
	}
	for _, value := range []cost.Basis{cost.BasisUnblended, cost.BasisAmortized, cost.BasisNet} {
		if !value.Valid() {
			t.Fatalf("invalid basis %s", value)
		}
	}
	if cost.Provider("other").Valid() || cost.Basis("other").Valid() {
		t.Fatal("accepted unknown enum")
	}
	if cost.NormalizeProvider("") != cost.ProviderCostExplorer || cost.NormalizeBasis("") != cost.BasisUnblended {
		t.Fatal("unexpected defaults")
	}
}
