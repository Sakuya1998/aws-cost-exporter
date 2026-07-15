// Package cost contains AWS cost domain values and their invariants.
package cost

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

var (
	// ErrInvalidAmount indicates an amount is malformed or non-finite.
	ErrInvalidAmount = errors.New("invalid monetary amount")
	// ErrEmptyCurrency indicates an amount has no currency unit.
	ErrEmptyCurrency = errors.New("currency must not be empty")
	// ErrMismatchedCurrency indicates two amounts use different units.
	ErrMismatchedCurrency = errors.New("currency mismatch")
)

// Money is a finite amount associated with a currency.
type Money struct {
	amount   float64
	currency string
}

// NewMoney validates and constructs a monetary value.
func NewMoney(amount float64, currency string) (Money, error) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return Money{}, ErrInvalidAmount
	}

	currency = strings.TrimSpace(currency)
	if currency == "" {
		return Money{}, ErrEmptyCurrency
	}

	return Money{amount: amount, currency: currency}, nil
}

// ParseMoney parses an AWS decimal amount and validates its currency.
func ParseMoney(amount, currency string) (Money, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(amount), 64)
	if err != nil {
		return Money{}, fmt.Errorf("%w: %v", ErrInvalidAmount, err)
	}

	return NewMoney(value, currency)
}

// Amount returns the monetary value as a Prometheus-compatible float.
func (money Money) Amount() float64 {
	return money.amount
}

// Currency returns the monetary unit reported by AWS.
func (money Money) Currency() string {
	return money.currency
}

// Add returns the sum of two amounts in the same currency.
func (money Money) Add(other Money) (Money, error) {
	if money.currency != other.currency {
		return Money{}, ErrMismatchedCurrency
	}
	sum := money.amount + other.amount
	if math.IsNaN(sum) || math.IsInf(sum, 0) {
		return Money{}, ErrInvalidAmount
	}
	return Money{amount: sum, currency: money.currency}, nil
}
