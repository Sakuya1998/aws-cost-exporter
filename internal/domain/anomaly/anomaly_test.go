package anomaly

import "testing"

func TestSort(t *testing.T) {
	values := []Summary{{Target: "z"}, {Target: "a"}}
	Sort(values)
	if values[0].Target != "a" {
		t.Fatalf("sort=%#v", values)
	}
}
