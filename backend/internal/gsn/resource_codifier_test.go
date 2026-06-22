package gsn

import (
	"os"
	"testing"
)

func TestResourceNumberKey(t *testing.T) {
	tests := map[string]string{
		"30059":  "30059",
		"M30059": "30059",
		"М91":    "91",
		"С12":    "12",
		"":       "",
	}
	for input, want := range tests {
		if got := resourceNumberKey(input); got != want {
			t.Fatalf("resourceNumberKey(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestExtractNormCodeFromCodifierField(t *testing.T) {
	tests := map[string]string{
		"####С1011-0101-0002":              "С1011-0101-0002",
		"С1542-0008":                       "С1542-0008",
		"###С1999-9999#":                   "С1999-9999",
		"С1411-9141##С1411-0021#С1411-0021": "С1411-9141",
		"":                                 "",
	}
	for input, want := range tests {
		if got := extractNormCodeFromCodifierField(input); got != want {
			t.Fatalf("extractNormCodeFromCodifierField(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseResourceCodifierFile(t *testing.T) {
	path := t.TempDir() + "/resurs.txt"
	content := "30059'19'####С1011-0101-0002'Детали фасонные'100 компл'0,93'*\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	rows, err := ParseResourceCodifierFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	row := rows[0]
	if row.Number != "30059" || row.Code != "С1011-0101-0002" || row.Name != "Детали фасонные" || row.Unit != "100 компл" {
		t.Fatalf("unexpected row: %#v", row)
	}
}
