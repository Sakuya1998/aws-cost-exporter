package identity

import "testing"

func TestCollectorIDValidationAndString(t *testing.T) {
	id := CollectorID{Target: "payer", Name: "total"}
	if !id.Valid() || id.String() != "payer/total" {
		t.Fatalf("id=%v valid=%v", id.String(), id.Valid())
	}
	for _, invalid := range []CollectorID{{}, {Target: " payer", Name: "total"}, {Target: "payer", Name: " total"}} {
		if invalid.Valid() {
			t.Fatalf("accepted %#v", invalid)
		}
	}
}
