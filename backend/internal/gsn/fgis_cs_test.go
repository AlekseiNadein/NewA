package gsn_test

import (
	"os"
	"path/filepath"
	"testing"

	"nav-saas-mvp/backend/internal/gsn"
)

func TestParseFGISPriceIndexFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	content := "З-0'''Заголовок'''*\r\n" +
		"А291-0101-014''14.3:2,22/14.11:2,30/29.1:1,64*\r\n" +
		"А291-0101-016''29.1:1,64*\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	rows, err := gsn.ParseFGISPriceIndexFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Code != "А291-0101-014" || rows[0].Value != "14.3:2,22/14.11:2,30/29.1:1,64" {
		t.Fatalf("unexpected first row: %#v", rows[0])
	}
}

func TestMergeFGISSetRows(t *testing.T) {
	dir := t.TempDir()
	pricesPath := filepath.Join(dir, "prices.txt")
	indexesPath := filepath.Join(dir, "indexes.txt")

	if err := os.WriteFile(pricesPath, []byte(
		"П0-0'''Цена'''*\n"+
			"А291-0101-016''14.3:4462,30/14.11:4695,26*\n"+
			"А291-0101-018''14.3:8957,19*\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexesPath, []byte(
		"З-0'''Индексы'''*\n"+
			"А291-0101-014''14.3:2,22*\n"+
			"А291-0101-016''29.1:1,64*\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	rows, err := gsn.MergeFGISSetRows(pricesPath, indexesPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 merged rows, got %d", len(rows))
	}

	byCode := make(map[string]gsn.FGISSetRow, len(rows))
	for _, row := range rows {
		byCode[row.Code] = row
	}

	if row := byCode["А291-0101-016"]; row.Prices == "" || row.Indexes == "" {
		t.Fatalf("expected merged row for А291-0101-016, got %#v", row)
	}
	if row := byCode["А291-0101-014"]; row.Indexes == "" || row.Prices != "" {
		t.Fatalf("expected indexes-only row for А291-0101-014, got %#v", row)
	}
	if row := byCode["А291-0101-018"]; row.Prices == "" || row.Indexes != "" {
		t.Fatalf("expected prices-only row for А291-0101-018, got %#v", row)
	}
}
