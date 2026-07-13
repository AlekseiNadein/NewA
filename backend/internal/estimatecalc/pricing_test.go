package estimatecalc

import (
	"testing"

	"nav-saas-mvp/backend/internal/gsn"
)

func TestResourceUnitPrice(t *testing.T) {
	tests := []struct {
		text, index string
		want        float64
	}{
		{"8957,19", "", 8957.19},
		{"451,93", "2,37", 1071.0741},
		{"100*2,09", "", 209},
	}
	for _, tc := range tests {
		got, err := ResourceUnitPrice(tc.text, tc.index)
		if err != nil {
			t.Fatalf("ResourceUnitPrice(%q, %q): %v", tc.text, tc.index, err)
		}
		if got != tc.want {
			t.Fatalf("ResourceUnitPrice(%q, %q) = %v, want %v", tc.text, tc.index, got, tc.want)
		}
	}
}

func TestLinePricingFromRecordResourcePosition(t *testing.T) {
	record := gsn.RecordDetail{
		Code:           "С1084-0303-0032",
		IsWork:         false,
		UnitPriceText:  "100,50",
		UnitPriceIndex: "2",
	}
	pricing, err := LinePricingFromRecord(record, 3)
	if err != nil {
		t.Fatal(err)
	}
	if pricing.UnitPrice != 201 {
		t.Fatalf("unit price = %v, want 201", pricing.UnitPrice)
	}
	if pricing.Total != 603 {
		t.Fatalf("total = %v, want 603", pricing.Total)
	}
}

func TestParseResourceQuantityPercentMarker(t *testing.T) {
	got, err := ParseResourceQuantity("П")
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("ParseResourceQuantity(П) = %v, want 0", got)
	}
}

func TestLinePricingFromRecordWorkPosition(t *testing.T) {
	record := gsn.RecordDetail{
		Code:   "Е0624-004-06",
		IsWork: true,
		Resources: []gsn.RecordResource{
			{Code: "r1", QuantityText: "1,5", UnitPriceText: "10"},
			{Code: "r2", QuantityText: "2", UnitPriceText: "5"},
			{Code: "r3", QuantityText: "П", UnitPriceText: "100"},
		},
	}
	pricing, err := LinePricingFromRecord(record, 4)
	if err != nil {
		t.Fatal(err)
	}
	wantTotal := (10*1.5 + 5*2) * 4
	if pricing.Total != wantTotal {
		t.Fatalf("total = %v, want %v", pricing.Total, wantTotal)
	}
	if pricing.UnitPrice != wantTotal/4 {
		t.Fatalf("unit price = %v, want %v", pricing.UnitPrice, wantTotal/4)
	}
}

func TestRoundMoney(t *testing.T) {
	tests := []struct {
		in, want float64
	}{
		{54.666666666666664, 54.67},
		{40.6809, 40.68},
		{77949.28632712, 77949.29},
	}
	for _, tc := range tests {
		if got := RoundMoney(tc.in); got != tc.want {
			t.Fatalf("RoundMoney(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
