package gsn

import "testing"

func TestExtractBasePrice(t *testing.T) {
	tests := map[string]string{
		"1-99:887,54#451,93":      "451,93",
		"1-99:35537,67##34458,33": "34458,33",
		"1-99:4381,87#514,69":     "514,69",
		"":                        "",
	}
	for input, want := range tests {
		if got := extractBasePrice(input); got != want {
			t.Fatalf("extractBasePrice(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeDistrictKey(t *testing.T) {
	tests := map[string]string{
		"14.3":  "14.3",
		"14.11": "14.11",
		"01":    "1",
		"1.5":   "1.5",
		"":      "",
	}
	for input, want := range tests {
		if got := normalizeDistrictKey(input); got != want {
			t.Fatalf("normalizeDistrictKey(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPickFGISValueByDistrict(t *testing.T) {
	values := "14.3:1361,38/14.11:1664,20/29.1:785,66"
	tests := map[string]string{
		"14.3":  "1361,38",
		"14.11": "1664,20",
		"29.1":  "785,66",
		"77":    "",
		"":      "",
	}
	for district, want := range tests {
		if got := pickFGISValueByDistrict(values, district); got != want {
			t.Fatalf("pickFGISValueByDistrict(%q, %q) = %q, want %q", values, district, got, want)
		}
	}
}

func TestResolveResourceUnitPrice(t *testing.T) {
	tests := []struct {
		prices, indexes, costIndicators, district string
		wantText, wantIndex                       string
	}{
		{
			prices:   "14.3:8957,19/14.11:9382,27",
			district: "14.3",
			wantText: "8957,19",
		},
		{
			indexes:        "14.3:2,09/14.11:2,37",
			costIndicators: "1-99:887,54#451,93",
			district:       "14.11",
			wantText:       "451,93",
			wantIndex:      "2,37",
		},
		{
			prices:          "14.3:100",
			indexes:         "14.3:2,09",
			costIndicators:  "1-99:887,54#451,93",
			district:        "14.3",
			wantText:        "100",
		},
		{
			indexes:        "14.3:2,09",
			costIndicators: "1-99:887,54#451,93",
			district:       "",
		},
		{
			prices:   "14.3:100",
			district: "29.1",
		},
	}
	for _, tc := range tests {
		gotText, gotIndex := resolveResourceUnitPrice(tc.prices, tc.indexes, tc.costIndicators, tc.district)
		if gotText != tc.wantText || gotIndex != tc.wantIndex {
			t.Fatalf("resolveResourceUnitPrice(%q, %q, %q, %q) = (%q, %q), want (%q, %q)",
				tc.prices, tc.indexes, tc.costIndicators, tc.district,
				gotText, gotIndex, tc.wantText, tc.wantIndex)
		}
	}
}
