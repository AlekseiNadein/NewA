package gsn

import "testing"

func TestApplyResourceNumberReplacementsInMemory(t *testing.T) {
	resources := []RecordResource{
		{Code: "С1000-0001-0001", Number: "11762", Name: "old", QuantityText: "1,5", UnitPriceText: "10"},
		{Code: "С1000-0001-0002", Number: "99999", Name: "keep", QuantityText: "2", UnitPriceText: "20"},
	}

	// Without DB: unresolved target keeps number code and original quantity.
	svc := &Service{}
	out, err := svc.ApplyResourceNumberReplacements(nil, resources, []ResourceNumberReplacement{
		{FromNumber: "11762", ToNumber: "6141"},
	}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0].Number != "6141" || out[0].Code != "6141" || out[0].QuantityText != "1,5" {
		t.Fatalf("replaced resource = %#v", out[0])
	}
	if out[1].Number != "99999" || out[1].Name != "keep" {
		t.Fatalf("untouched resource = %#v", out[1])
	}
}
