package estimatecalc

import (
	"testing"

	"nav-saas-mvp/backend/internal/gsn"
)

func TestBuildLineCalcSnapshotWork(t *testing.T) {
	record := gsn.RecordDetail{
		Code:         "Е0624-004-06",
		OriginalCode: "Е0624-004-06",
		Name:         "Работа",
		Unit:         "м3",
		IsWork:       true,
		Resources: []gsn.RecordResource{
			{Code: "С1111-0301-0062", Name: "Песок", Unit: "м3", QuantityText: "1,5", UnitPriceText: "10", Determinant: "A"},
			{Code: "С1111-0301-0062", Name: "Песок", Unit: "м3", QuantityText: "0,5", UnitPriceText: "10", Determinant: "A"},
			{Code: "М1234-0001", Name: "Машина", Unit: "маш.-ч", QuantityText: "2", UnitPriceText: "5"},
		},
	}
	snap, err := BuildLineCalcSnapshot(record, 4)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Total != (10*2+5*2)*4 {
		t.Fatalf("total = %v", snap.Total)
	}
	if len(snap.Resources) != 2 {
		t.Fatalf("resources = %d, want 2 merged", len(snap.Resources))
	}
	if snap.Resources[0].Consumption != 8 {
		t.Fatalf("first consumption = %v, want 8", snap.Resources[0].Consumption)
	}
	if snap.ResourcesText != "С1111-0301-0062.8/М1234-0001.8" {
		t.Fatalf("resourcesText = %q", snap.ResourcesText)
	}
	if snap.Determinant != "" {
		t.Fatalf("work line determinant = %q, want empty", snap.Determinant)
	}
}

func TestBuildLineCalcSnapshotResourcePosition(t *testing.T) {
	record := gsn.RecordDetail{
		Code:           "С1084-0303-0032",
		Name:           "Ресурс",
		Unit:           "т",
		IsWork:         false,
		UnitPriceText:  "100,50",
		UnitPriceIndex: "2",
		Determinant:    "X",
		Mass:           "1,2",
	}
	snap, err := BuildLineCalcSnapshot(record, 3)
	if err != nil {
		t.Fatal(err)
	}
	if snap.UnitPrice != 201 || snap.Total != 603 {
		t.Fatalf("pricing unit=%v total=%v", snap.UnitPrice, snap.Total)
	}
	if len(snap.Resources) != 1 {
		t.Fatalf("resources = %d", len(snap.Resources))
	}
	if snap.Resources[0].Consumption != 3 || snap.Resources[0].Determinant != "X" {
		t.Fatalf("resource = %+v", snap.Resources[0])
	}
	if snap.Determinant != "X" {
		t.Fatalf("line determinant = %q, want X", snap.Determinant)
	}
	if snap.ResourcesText != "С1084-0303-0032.3" {
		t.Fatalf("resourcesText = %q", snap.ResourcesText)
	}
}

func TestApplyDeterminantAssignment(t *testing.T) {
	snap := LineCalcSnapshot{
		Code:        "С1084-0303-0032",
		Determinant: "X",
		Resources: []ResourceContribution{
			{Code: "С1084-0303-0032", Determinant: "X", Consumption: 3},
			{Code: "М1234-0001", Determinant: "A", Consumption: 1},
		},
	}
	ApplyDeterminantAssignment(&snap, "14")
	if snap.Determinant != "14" {
		t.Fatalf("line determinant = %q, want 14", snap.Determinant)
	}
	if snap.Resources[0].Determinant != "14" {
		t.Fatalf("self resource determinant = %q, want 14", snap.Resources[0].Determinant)
	}
	if snap.Resources[1].Determinant != "A" {
		t.Fatalf("nested resource determinant = %q, want A", snap.Resources[1].Determinant)
	}

	ApplyDeterminantAssignment(&snap, "  ")
	if snap.Determinant != "14" {
		t.Fatalf("empty assignment should not clear determinant, got %q", snap.Determinant)
	}
}

func TestFormatResourcesText(t *testing.T) {
	got := FormatResourcesText([]ResourceContribution{
		{Code: "A", Consumption: 1.5},
		{Code: "B", Consumption: 0},
	})
	if got != "A.1,5/B.0" {
		t.Fatalf("got %q", got)
	}
}
