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

// TestMoneyAddSumsSameCurrency verifies month-to-date aggregation can combine
// daily amounts without changing the currency unit.
func TestMoneyAddSumsSameCurrency(t *testing.T) {
	t.Parallel()

	left, err := cost.NewMoney(10.5, "USD")
	if err != nil {
		t.Fatalf("NewMoney(left) error = %v", err)
	}
	right, err := cost.NewMoney(20.25, "USD")
	if err != nil {
		t.Fatalf("NewMoney(right) error = %v", err)
	}
	sum, err := left.Add(right)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got := sum.Amount(); got != 30.75 {
		t.Fatalf("Amount() = %v, want 30.75", got)
	}
	if got := sum.Currency(); got != "USD" {
		t.Fatalf("Currency() = %q, want USD", got)
	}
}

// TestMoneyAddRejectsMismatchedCurrency verifies cross-currency sums are
// rejected before they reach snapshots or Prometheus export.
func TestMoneyAddRejectsMismatchedCurrency(t *testing.T) {
	t.Parallel()

	left, _ := cost.NewMoney(1, "USD")
	right, _ := cost.NewMoney(1, "EUR")
	if _, err := left.Add(right); !errors.Is(err, cost.ErrMismatchedCurrency) {
		t.Fatalf("Add() error = %v, want ErrMismatchedCurrency", err)
	}
}

// TestMoneyAddRejectsNonFiniteSum verifies overflow to non-finite values is
// rejected like direct construction.
func TestMoneyAddRejectsNonFiniteSum(t *testing.T) {
	t.Parallel()

	left, _ := cost.NewMoney(math.MaxFloat64, "USD")
	right, _ := cost.NewMoney(math.MaxFloat64, "USD")
	if _, err := left.Add(right); !errors.Is(err, cost.ErrInvalidAmount) {
		t.Fatalf("Add() error = %v, want ErrInvalidAmount", err)
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
