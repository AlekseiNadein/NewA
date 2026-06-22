package gsn

import "testing"

func TestIsWorkNormRecord(t *testing.T) {
	tests := []struct {
		code, kind string
		want       bool
	}{
		{"Е0101-001-01", "norm", true},
		{"У0101-001-01", "norm", true},
		{"С291-0101-035", "resource_catalog", false},
		{"С1011-0101-0002", "resource_catalog", false},
		{"Ц123-001-01", "coefficient_catalog", false},
		{"Ц123-001-01", "norm", true},
	}
	for _, tc := range tests {
		if got := isWorkNormRecord(tc.code, tc.kind); got != tc.want {
			t.Fatalf("isWorkNormRecord(%q, %q) = %v, want %v", tc.code, tc.kind, got, tc.want)
		}
	}
}

func TestIsResourcePositionCode(t *testing.T) {
	tests := map[string]bool{
		"С291-0101-035": true,
		"М30059":        true,
		"Е0101-001-01":  false,
		"":              false,
	}
	for code, want := range tests {
		if got := isResourcePositionCode(code); got != want {
			t.Fatalf("isResourcePositionCode(%q) = %v, want %v", code, got, want)
		}
	}
}
