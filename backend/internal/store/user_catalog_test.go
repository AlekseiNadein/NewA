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
	if got := SourceDataDeterminantFromRawText(raw); got != "14" {
		t.Fatalf("determinant = %q, want 14", got)
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

func TestExtractSourceDataDeterminantAssignment(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"ТПрайс-лист(=14)", "14"},
		{"СТПрайс подрядчика(=13)", "13"},
		{"С1084-0303-0032(=12)", "12"},
		{"С1084-0303-0032 (KLink=Е0624-003-02)", ""},
		{"Е0624-004-06 (РМ59092РМ60193)", ""},
		{"Е0624-004-06 (РМ1)(=9)", "9"},
		{"CODE", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := ExtractSourceDataDeterminantAssignment(tc.in); got != tc.want {
			t.Fatalf("ExtractSourceDataDeterminantAssignment(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestExtractSourceDataResourceReplacements(t *testing.T) {
	cases := []struct {
		in   string
		want []SourceDataResourceReplacement
	}{
		{
			"Е0801-002-02 (РМ11762РМ6141)",
			[]SourceDataResourceReplacement{{FromNumber: "11762", ToNumber: "6141"}},
		},
		{
			"Е0624-002-05 (РМ24214РМ58316=0,1)",
			[]SourceDataResourceReplacement{{FromNumber: "24214", ToNumber: "58316", AbsoluteQuantity: "0,1"}},
		},
		{
			"Е0619-005-02 (РМ24214РМ58316=1)(РМ11767РМ60208)(РМ34239РМ8391)",
			[]SourceDataResourceReplacement{
				{FromNumber: "24214", ToNumber: "58316", AbsoluteQuantity: "1"},
				{FromNumber: "11767", ToNumber: "60208"},
				{FromNumber: "34239", ToNumber: "8391"},
			},
		},
		{
			"Е0802-017-01 (РС22461РС22451)(РМ33840РС22787)",
			[]SourceDataResourceReplacement{
				{FromNumber: "22461", ToNumber: "22451"},
				{FromNumber: "33840", ToNumber: "22787"},
			},
		},
		{
			"Е0000 (РМ31434Р45731=0,0130)",
			[]SourceDataResourceReplacement{{FromNumber: "31434", ToNumber: "45731", AbsoluteQuantity: "0,0130"}},
		},
		{
			"Е0000 (РМ11762РМ6141.0,5)",
			[]SourceDataResourceReplacement{{FromNumber: "11762", ToNumber: "6141", Coefficient: 0.5, HasCoefficient: true}},
		},
		{
			"Е0000 (РМ11762РМ6141.2)",
			[]SourceDataResourceReplacement{{FromNumber: "11762", ToNumber: "6141", Coefficient: 2, HasCoefficient: true}},
		},
		{"Е0624-003-02 (РМ34239)", nil},
		{"С1084-0303-0032 (KLink=Е0624-003-02)", nil},
		{"ТПрайс-лист(=14)", nil},
		{"Е0624-004-06 (РМ1)(=9)", nil},
	}
	for _, tc := range cases {
		got := ExtractSourceDataResourceReplacements(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("ExtractSourceDataResourceReplacements(%q) len=%d want %d (%v)", tc.in, len(got), len(tc.want), got)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("ExtractSourceDataResourceReplacements(%q)[%d] = %#v, want %#v", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestExtractSourceDataResourceDeletions(t *testing.T) {
	cases := []struct {
		in   string
		want []SourceDataResourceDeletion
	}{
		{
			"Е0624-003-02 (РМ34239)",
			[]SourceDataResourceDeletion{{Number: "34239"}},
		},
		{
			"Е0624-004-06 (РМ1)(=9)",
			[]SourceDataResourceDeletion{{Number: "1"}},
		},
		{
			"Е0619-005-02 (РМ24214РМ58316=1)(РМ11767РМ60208)(РМ34239)",
			[]SourceDataResourceDeletion{{Number: "34239"}},
		},
		{
			"Е0000 (РМ31434=0,0130)",
			[]SourceDataResourceDeletion{{Number: "31434"}},
		},
		{"Е0801-002-02 (РМ11762РМ6141)", nil},
		{"С1084-0303-0032 (KLink=Е0624-003-02)", nil},
		{"ТПрайс-лист(=14)", nil},
	}
	for _, tc := range cases {
		got := ExtractSourceDataResourceDeletions(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("ExtractSourceDataResourceDeletions(%q) len=%d want %d (%v)", tc.in, len(got), len(tc.want), got)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("ExtractSourceDataResourceDeletions(%q)[%d] = %#v, want %#v", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestSourceDataResourceReplacementsFromRawText(t *testing.T) {
	raw := "Е0801-002-02 (РМ11762РМ6141)'(61,475)[4]''Устройство основания'м3"
	got := SourceDataResourceReplacementsFromRawText(raw)
	if len(got) != 1 || got[0].FromNumber != "11762" || got[0].ToNumber != "6141" {
		t.Fatalf("unexpected replacements: %#v", got)
	}
}

func TestSourceDataResourceDeletionsFromRawText(t *testing.T) {
	raw := "Е0624-003-02 (РМ34239)'(1)''Удаление ресурса'м3"
	got := SourceDataResourceDeletionsFromRawText(raw)
	if len(got) != 1 || got[0].Number != "34239" {
		t.Fatalf("unexpected deletions: %#v", got)
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
