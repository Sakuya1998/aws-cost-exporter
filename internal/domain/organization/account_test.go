package organization

import "testing"

func TestSortOrdersByTargetAndAccount(t *testing.T) {
	values := []Account{{Target: "b", AccountID: "1"}, {Target: "a", AccountID: "2"}, {Target: "a", AccountID: "1"}}
	Sort(values)
	if values[0].Target != "a" || values[0].AccountID != "1" || values[2].Target != "b" {
		t.Fatalf("sorted=%#v", values)
	}
}
