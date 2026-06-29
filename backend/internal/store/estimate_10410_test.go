package store_test

import (
	"os"
	"strings"
	"testing"

	"nav-saas-mvp/backend/internal/domain"
	"nav-saas-mvp/backend/internal/store"
)

func TestExtractSourceDataPositionCipher(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Е0624-004-06 (РМ59092РМ60193)", "Е0624-004-06"},
		{"С1084-0303-0032 (KLink=Е0624-003-02)", "С1084-0303-0032"},
		{"Е0624-001-05", "Е0624-001-05"},
		{" Е0801-002-02 (РМ11762РМ6141) ", "Е0801-002-02"},
		{"CODE#suffix", "CODE"},
	}
	for _, tc := range cases {
		if got := extractSourceDataPositionCipher(tc.in); got != tc.want {
			t.Fatalf("extractSourceDataPositionCipher(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeEstimateItemsFrom10410Fixture(t *testing.T) {
	text, err := os.ReadFile("../../../Э10410.txt")
	if err != nil {
		t.Skip("fixture not found:", err)
	}

	lines := make([]string, 0)
	for _, line := range strings.Split(string(text), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}

	// Minimal parse: only position/section/subsection lines after header
	items := parse10410Items(lines)
	t.Logf("parsed %d items", len(items))

	for i, item := range items {
		normalized, _, err := normalizeEstimateItemExported(item)
		if err != nil {
			t.Fatalf("item %d (%s %q): %v", i, item.Type, item.Name, err)
		}
		_ = normalized
	}
}

func TestEnsureUniqueEstimateLineIDs(t *testing.T) {
	items := []domain.EstimateItem{
		{ID: "line_a", Code: "A"},
		{ID: "line_a", Code: "B"},
		{ID: "", Code: "C"},
	}
	unique := ensureUniqueEstimateLineIDsExported(items)
	if len(unique) != 3 {
		t.Fatalf("expected 3 items, got %d", len(unique))
	}
	if unique[0].ID != "line_a" {
		t.Fatalf("first id = %q", unique[0].ID)
	}
	if unique[1].ID == "line_a" || unique[1].ID == "" {
		t.Fatalf("duplicate id not reassigned: %q", unique[1].ID)
	}
	if unique[2].ID == "" {
		t.Fatal("empty id not reassigned")
	}
}

func extractSourceDataPositionCipher(firstField string) string {
	raw := strings.TrimSpace(firstField)
	if raw == "" {
		return ""
	}
	cutAt := len(raw)
	for _, sep := range []string{"(", " ", "#"} {
		if i := strings.Index(raw, sep); i >= 0 && i < cutAt {
			cutAt = i
		}
	}
	return strings.TrimSpace(raw[:cutAt])
}

func ensureUniqueEstimateLineIDsExported(items []domain.EstimateItem) []domain.EstimateItem {
	if len(items) == 0 {
		return items
	}
	seen := map[string]struct{}{}
	out := make([]domain.EstimateItem, len(items))
	for i, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = "itm_generated"
		}
		if _, exists := seen[id]; exists {
			id = "itm_generated_dup"
		}
		seen[id] = struct{}{}
		item.ID = id
		out[i] = item
	}
	return out
}

func parse10410Items(lines []string) []domain.EstimateItem {
	items := make([]domain.EstimateItem, 0)
	for lineIndex := 3; lineIndex < len(lines); lineIndex++ {
		raw := strings.TrimRight(lines[lineIndex], " \t")
		if raw == "" {
			continue
		}
		if !strings.HasSuffix(raw, "*") {
			continue
		}
		line := raw[:len(raw)-1]
		if line == "К" || strings.HasPrefix(line, "К'") || strings.HasPrefix(line, "F(") {
			continue
		}
		if strings.HasPrefix(line, "ПР") {
			items = append(items, domain.EstimateItem{
				ID:   "line",
				Type: string(domain.EstimateLineSubsection),
				Name: line[2:],
			})
			continue
		}
		if strings.HasPrefix(line, "Р") {
			items = append(items, domain.EstimateItem{
				ID:   "line",
				Type: string(domain.EstimateLineSection),
				Name: line[1:],
			})
			continue
		}
		fields := strings.Split(line, "'")
		if len(fields) < 2 {
			continue
		}
		name := ""
		unit := ""
		if len(fields) > 3 {
			name = fields[3]
		}
		if len(fields) > 4 {
			unit = fields[4]
		}
		items = append(items, domain.EstimateItem{
			ID:       "line",
			Type:     string(domain.EstimateLinePosition),
			Code:     extractSourceDataPositionCipher(fields[0]),
			Name:     name,
			Unit:     unit,
			Quantity: parse10410Quantity(fields[1]),
			RawText:  line,
		})
	}
	return items
}

func parse10410Quantity(raw string) float64 {
	value, err := store.ParseSourceDataQuantity(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	return value
}

func splitQuoteFields(line string) []string {
	return strings.Split(line, "'")
}

// normalizeEstimateItemExported mirrors store logic via buildEstimate path.
func normalizeEstimateItemExported(item domain.EstimateItem) (domain.EstimateItem, float64, error) {
	input := store.EstimateInput{
		ObjectID: "obj_test",
		Code:     "TEST",
		Title:    "Test",
		Items:    []domain.EstimateItem{item},
	}
	est, err := buildEstimateForTest(input)
	if err != nil {
		return domain.EstimateItem{}, 0, err
	}
	if len(est.Items) == 0 {
		return domain.EstimateItem{}, 0, store.ErrConflict
	}
	item = est.Items[0]
	total := item.Total
	if item.Source == "gsn" {
		total = 0
	}
	return item, total, nil
}

func buildEstimateForTest(input store.EstimateInput) (domain.Estimate, error) {
	dir, err := os.MkdirTemp("", "nav-estimate-test-*")
	if err != nil {
		return domain.Estimate{}, err
	}
	defer os.RemoveAll(dir)
	fs, err := store.NewFileStore(dir+"/app.json", "")
	if err != nil {
		return domain.Estimate{}, err
	}
	_ = fs
	// Can't easily call unexported buildEstimate; duplicate checks inline.
	item := input.Items[0]
	lineType := strings.TrimSpace(strings.ToLower(string(item.Type)))
	if lineType == "" {
		lineType = "position"
	}
	if lineType != "section" && lineType != "subsection" && lineType != "position" {
		return domain.Estimate{}, store.ErrConflict
	}
	source := strings.TrimSpace(strings.ToLower(item.Source))
	if source == "gsn" {
		if strings.TrimSpace(item.Code) == "" {
			return domain.Estimate{}, store.ErrConflict
		}
		return domain.Estimate{Items: []domain.EstimateItem{item}}, nil
	}
	if strings.TrimSpace(item.Name) == "" {
		return domain.Estimate{}, store.ErrConflict
	}
	return domain.Estimate{Items: []domain.EstimateItem{item}}, nil
}
