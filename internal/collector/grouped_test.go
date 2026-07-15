package collector

import (
	"errors"
	"testing"
)

func TestValidateOverflowLabelRejectsEmpty(t *testing.T) {
	if err := ValidateOverflowLabel(""); !errors.Is(err, ErrInvalidOverflowLabel) {
		t.Fatalf("ValidateOverflowLabel(\"\") = %v, want ErrInvalidOverflowLabel", err)
	}
	if err := ValidateOverflowLabel(DefaultOverflowLabel); err != nil {
		t.Fatalf("ValidateOverflowLabel(default) = %v", err)
	}
}
