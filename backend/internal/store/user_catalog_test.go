package store

import "testing"

func TestIsUserCatalogCipherCode(t *testing.T) {
	tests := map[string]bool{
		"ТПрайс-лист":           true,
		"ТПрайс-лист(=14)":      true,
		"СТПрайс":               true,
		"СТПрайс подрядчика(=13)": true,
		"С1243-0516-0131":       false,
		"Е1803-008-01":          false,
		"":                      false,
	}
	for code, want := range tests {
		if got := IsUserCatalogCipherCode(code); got != want {
			t.Fatalf("IsUserCatalogCipherCode(%q) = %v, want %v", code, got, want)
		}
	}
}

func TestParseSourceDataPositionFieldsWithIndexMarker(t *testing.T) {
	raw := "ТПрайс-лист(=14)'(1)[4]'359'Диффузор DVS Ф100 мм'шт"
	fields, err := ParseSourceDataPositionFields(raw)
	if err != nil {
		t.Fatal(err)
	}
	if fields.SourceCode != "ТПрайс-лист" {
		t.Fatalf("source code = %q", fields.SourceCode)
	}
	if !fields.HasTotal || fields.Total != 359 {
		t.Fatalf("total = %v hasTotal=%v", fields.Total, fields.HasTotal)
	}
	if !fields.HasName || fields.Name != "Диффузор DVS Ф100 мм" {
		t.Fatalf("name = %q hasName=%v", fields.Name, fields.HasName)
	}
	if !fields.HasUnit || fields.Unit != "шт" {
		t.Fatalf("unit = %q hasUnit=%v", fields.Unit, fields.HasUnit)
	}
	if UserCatalogPositionNeedsLookup(fields) {
		t.Fatal("expected complete source data, lookup not needed")
	}
}

func TestUserCatalogPositionNeedsLookup(t *testing.T) {
	complete, err := ParseSourceDataPositionFields("ТПрайс-лист'(1)'359'Диффузор'шт")
	if err != nil {
		t.Fatal(err)
	}
	if UserCatalogPositionNeedsLookup(complete) {
		t.Fatal("complete line should not need lookup")
	}

	partial, err := ParseSourceDataPositionFields("ТПрайс-лист'(1)")
	if err != nil {
		t.Fatal(err)
	}
	if !UserCatalogPositionNeedsLookup(partial) {
		t.Fatal("line without trailing fields should need lookup")
	}
}
