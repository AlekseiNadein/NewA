package store

import (
	"math"
	"os"
	"strings"
	"testing"

	"nav-saas-mvp/backend/internal/domain"
)

func TestSplitSourceDataFieldsSample10410(t *testing.T) {
	line := "Е0801-002-02 (РМ11762РМ6141)'(61,475)[4]''Устройство основания под фундаменты: щебеночного'м3"
	fields := SplitSourceDataFields(line)
	if len(fields) < 4 {
		t.Fatalf("fields = %#v", fields)
	}
	if fields[0] != "Е0801-002-02 (РМ11762РМ6141)" {
		t.Fatalf("code field = %q", fields[0])
	}
	if fields[1] != "(61,475)[4]" {
		t.Fatalf("quantity field = %q", fields[1])
	}
	if fields[2] != "" {
		t.Fatalf("total field = %q", fields[2])
	}
	if fields[3] != "Устройство основания под фундаменты: щебеночного" {
		t.Fatalf("name field = %q", fields[3])
	}
}

func TestParseSourceDataQuantity(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"(61,475)[4]", 61.475},
		{"(5,76)[4]", 5.76},
		{"(1,008)[4]", 1.008},
		{"(213)[4]", 213},
		{"(0,0946944)[4]", 0.0947},
		{"(5+3,5)*2[2]", 17},
		{"(10-2)/4[3]", 2},
		{"(2.3)[0]", 6},
		{"(10:2)[0]", 5},
		{"(5,5:2)[2]", 2.75},
		{"0,16.(3,68)", 0.5888},
		{"5,76", 5.76},
		{"", 0},
	}
	for _, tc := range cases {
		got, err := ParseSourceDataQuantity(tc.in)
		if err != nil {
			t.Fatalf("ParseSourceDataQuantity(%q) error: %v", tc.in, err)
		}
		if math.Abs(got-tc.want) > 1e-9 {
			t.Fatalf("ParseSourceDataQuantity(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseSourceDataQuantityFrom10410Fixture(t *testing.T) {
	text, err := os.ReadFile("../../../Э10410.txt")
	if err != nil {
		t.Skip("fixture not found:", err)
	}

	for lineIndex, line := range strings.Split(string(text), "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !strings.HasSuffix(strings.TrimRight(line, " \t"), "*") {
			continue
		}
		raw := strings.TrimRight(line, " \t")
		raw = raw[:len(raw)-1]
		if lineIndex < 3 || strings.HasPrefix(raw, "ПР") || strings.HasPrefix(raw, "Р") || raw == "К" || strings.HasPrefix(raw, "К'") || strings.HasPrefix(raw, "F(") || strings.HasPrefix(raw, "Э") || strings.HasPrefix(raw, "Ю") {
			continue
		}
		fields := SplitSourceDataFields(raw)
		if len(fields) < 2 {
			continue
		}
		quantityRaw := strings.TrimSpace(fields[1])
		if quantityRaw == "" {
			continue
		}
		value, err := ParseSourceDataQuantity(quantityRaw)
		if err != nil {
			t.Fatalf("line %d quantity %q: %v", lineIndex+1, quantityRaw, err)
		}
		if value <= 0 {
			t.Fatalf("line %d quantity %q parsed to %v", lineIndex+1, quantityRaw, value)
		}
	}
}

func TestQuantityFromSourceDataLine(t *testing.T) {
	raw := "Е0801-002-02 (РМ11762РМ6141)'(61,475)[4]''Устройство основания под фундаменты: щебеночного'м3"
	value, err := quantityFromSourceDataLine(raw)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(value-61.475) > 1e-9 {
		t.Fatalf("quantity = %v, want 61.475", value)
	}
}

func TestParseSourceDataQuantityFrom10420Fixture(t *testing.T) {
	text, err := os.ReadFile("../../../Э10420.txt")
	if err != nil {
		t.Skip("fixture not found:", err)
	}

	for lineIndex, line := range strings.Split(string(text), "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !strings.HasSuffix(strings.TrimRight(line, " \t"), "*") {
			continue
		}
		raw := strings.TrimRight(line, " \t")
		raw = raw[:len(raw)-1]
		if lineIndex < 3 || strings.HasPrefix(raw, "ПР") || strings.HasPrefix(raw, "Р") || raw == "К" || strings.HasPrefix(raw, "К'") || strings.HasPrefix(raw, "F(") || strings.HasPrefix(raw, "Э") || strings.HasPrefix(raw, "Ю") {
			continue
		}
		fields := SplitSourceDataFields(raw)
		if len(fields) < 2 {
			continue
		}
		quantityRaw := strings.TrimSpace(fields[1])
		if quantityRaw == "" {
			continue
		}
		value, err := ParseSourceDataQuantity(quantityRaw)
		if err != nil {
			t.Fatalf("line %d quantity %q: %v", lineIndex+1, quantityRaw, err)
		}
		if value <= 0 {
			t.Fatalf("line %d quantity %q parsed to %v", lineIndex+1, quantityRaw, value)
		}
	}
}

func TestNormalizeEstimateItemQuantityFrom10420Line(t *testing.T) {
	item := domain.EstimateItem{
		Type:    string(domain.EstimateLinePosition),
		Source:  "gsn",
		Code:    "С1111-301-62",
		RawText: "С1111-301-62(К Link=Е1001-2-1 LinkR=37930)'0,16.(3,68)''Бруски обрезные хвойных пород (ель, сосна), естественной влажности, длина 2-6,5 м, ширина 20-90 мм, толщина 20-90 мм, сорт II'м3",
	}
	normalized, _, err := normalizeEstimateItem(item)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(normalized.Quantity-0.5888) > 1e-4 {
		t.Fatalf("quantity = %v, want 0.5888", normalized.Quantity)
	}
}

func TestNormalizeEstimateItemQuantityFromRawText(t *testing.T) {
	item := domain.EstimateItem{
		Type:    string(domain.EstimateLinePosition),
		Source:  "gsn",
		Code:    "Е0801-002-02 (РМ11762РМ6141)",
		RawText: "Е0801-002-02 (РМ11762РМ6141)'(61,475)[4]''Устройство основания под фундаменты: щебеночного'м3",
	}
	normalized, _, err := normalizeEstimateItem(item)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Code != "Е0801-002-02" {
		t.Fatalf("code = %q", normalized.Code)
	}
	if math.Abs(normalized.Quantity-61.475) > 1e-9 {
		t.Fatalf("quantity = %v, want 61.475", normalized.Quantity)
	}
}
