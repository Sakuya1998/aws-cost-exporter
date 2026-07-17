package budget

import "testing"

func TestSortOrdersByTargetAndName(t *testing.T) {
	values := []Budget{{Target: "b", Name: "a"}, {Target: "a", Name: "z"}, {Target: "a", Name: "a"}}
	Sort(values)
	if values[0].Target != "a" || values[0].Name != "a" || values[2].Target != "b" {
		t.Fatalf("sorted=%#v", values)
	}
}
