package gsn

import "testing"

func TestNormalizeResourceCatalogCode(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"С1111-301-62", "С1111-0301-0062"},
		{"С1111-0301-0062", "С1111-0301-0062"},
		{"Е1001-039-01", "Е1001-039-01"},
		{"С1017-0407-У105", "С1017-0407-У105"},
		{"", ""},
	}
	for _, tc := range tests {
		got := NormalizeResourceCatalogCode(tc.in)
		if got != tc.want {
			t.Fatalf("NormalizeResourceCatalogCode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRecordLookupCandidates(t *testing.T) {
	got := recordLookupCandidates("С1111-301-62")
	if len(got) != 2 || got[0] != "С1111-301-62" || got[1] != "С1111-0301-0062" {
		t.Fatalf("recordLookupCandidates = %#v", got)
	}
}
