package cost_test

import (
	"errors"
	"math"
	"testing"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

// TestNewMoneyStoresValidValues verifies credits and normalized currencies are
// represented without changing the amount.
func TestNewMoneyStoresValidValues(t *testing.T) {
	t.Parallel()

	money, err := cost.NewMoney(-12.5, " USD ")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}

	if got := money.Amount(); got != -12.5 {
		t.Errorf("Amount() = %v, want %v", got, -12.5)
	}
	if got := money.Currency(); got != "USD" {
		t.Errorf("Currency() = %q, want %q", got, "USD")
	}
}

// TestNewMoneyRejectsNonFiniteAmount verifies values unsupported by Prometheus
// cannot enter the cost domain.
func TestNewMoneyRejectsNonFiniteAmount(t *testing.T) {
	t.Parallel()

	for _, amount := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := cost.NewMoney(amount, "USD"); !errors.Is(err, cost.ErrInvalidAmount) {
			t.Errorf("NewMoney(%v) error = %v, want ErrInvalidAmount", amount, err)
		}
	}
}

// TestNewMoneyRejectsBlankCurrency verifies a cost always carries its unit.
func TestNewMoneyRejectsBlankCurrency(t *testing.T) {
	t.Parallel()

	if _, err := cost.NewMoney(1, " \t"); !errors.Is(err, cost.ErrEmptyCurrency) {
		t.Fatalf("NewMoney() error = %v, want ErrEmptyCurrency", err)
	}
}

// TestParseMoneyParsesCostExplorerDecimal verifies AWS decimal strings are
// converted through the same domain validation as direct values.
func TestParseMoneyParsesCostExplorerDecimal(t *testing.T) {
	t.Parallel()

	money, err := cost.ParseMoney("1234.56", "USD")
	if err != nil {
		t.Fatalf("ParseMoney() returned an unexpected error: %v", err)
	}
	if got := money.Amount(); got != 1234.56 {
		t.Fatalf("Amount() = %v, want %v", got, 1234.56)
	}
}

// TestParseMoneyRejectsInvalidDecimal verifies malformed AWS amounts do not
// reach snapshots.
func TestParseMoneyRejectsInvalidDecimal(t *testing.T) {
	t.Parallel()

	if _, err := cost.ParseMoney("not-a-number", "USD"); !errors.Is(err, cost.ErrInvalidAmount) {
		t.Fatalf("ParseMoney() error = %v, want ErrInvalidAmount", err)
	}
}
