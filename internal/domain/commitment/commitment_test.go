package commitment

import "testing"

func TestTypeAndSort(t *testing.T) {
	if !TypeSavingsPlan.Valid() || !TypeReservation.Valid() || Type("other").Valid() {
		t.Fatal("invalid type contract")
	}
	values := []Summary{{Target: "z", Type: TypeReservation}, {Target: "a", Type: TypeSavingsPlan}}
	Sort(values)
	if values[0].Target != "a" {
		t.Fatalf("sort=%#v", values)
	}
}

func TestSortUsesTypeAndTimeUnitAsTieBreakers(t *testing.T) {
	values := []Summary{
		{Target: "payer", Type: TypeReservation, TimeUnit: "MONTHLY"},
		{Target: "payer", Type: TypeSavingsPlan, TimeUnit: "YEARLY"},
		{Target: "payer", Type: TypeSavingsPlan, TimeUnit: "MONTHLY"},
	}
	Sort(values)
	if values[0].Type != TypeReservation || values[1].TimeUnit != "MONTHLY" || values[2].TimeUnit != "YEARLY" {
		t.Fatalf("sorted=%#v", values)
	}
}
