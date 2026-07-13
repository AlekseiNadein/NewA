package store

import (
	"fmt"
	"strconv"
	"strings"
)

// IsUserCatalogCipherCode reports whether the cipher should be resolved from user
// positions instead of the GSN normative base (first or second rune is Т/T).
func IsUserCatalogCipherCode(code string) bool {
	code = strings.TrimSpace(extractSourceDataPositionCipher(code))
	runes := []rune(code)
	if len(runes) == 0 {
		return false
	}
	switch runes[0] {
	case 'Т', 'T':
		return true
	}
	if len(runes) >= 2 {
		switch runes[1] {
		case 'Т', 'T':
			return true
		}
	}
	return false
}

// SourceDataPositionFields are optional trailing fields of a source-data position line.
type SourceDataPositionFields struct {
	SourceCode string
	HasTotal   bool
	Total      float64
	HasName    bool
	Name       string
	HasUnit    bool
	Unit       string
}

func isSourceDataIndexMarkerField(raw string) bool {
	raw = strings.TrimSpace(raw)
	if len(raw) < 3 {
		return false
	}
	return raw[0] == '[' && raw[len(raw)-1] == ']'
}

func sourceDataPositionValueIndexes(fields []string) (totalIdx, nameIdx, unitIdx int) {
	if len(fields) > 2 && isSourceDataIndexMarkerField(fields[2]) {
		return 3, 4, 5
	}
	return 2, 3, 4
}

func parseSourceDataLocalizedNumber(raw string) (float64, error) {
	normalized := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(raw), " ", ""), ",", ".")
	if normalized == "" {
		return 0, nil
	}
	value, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", raw)
	}
	return value, nil
}

// ParseSourceDataPositionFields parses optional cost/name/unit from a source-data line.
func ParseSourceDataPositionFields(rawText string) (SourceDataPositionFields, error) {
	fields := sourceDataLineFields(rawText)
	if fields == nil {
		return SourceDataPositionFields{}, fmt.Errorf("invalid source data line")
	}

	out := SourceDataPositionFields{
		SourceCode: extractSourceDataPositionCipher(fields[0]),
	}
	totalIdx, nameIdx, unitIdx := sourceDataPositionValueIndexes(fields)

	if totalIdx < len(fields) {
		totalRaw := strings.TrimSpace(fields[totalIdx])
		if totalRaw != "" {
			total, err := parseSourceDataLocalizedNumber(totalRaw)
			if err != nil {
				return SourceDataPositionFields{}, err
			}
			out.HasTotal = true
			out.Total = total
		}
	}
	if nameIdx < len(fields) {
		out.Name = strings.TrimSpace(fields[nameIdx])
		out.HasName = out.Name != ""
	}
	if unitIdx < len(fields) {
		out.Unit = strings.TrimSpace(fields[unitIdx])
		out.HasUnit = out.Unit != ""
	}
	return out, nil
}

// UserCatalogPositionNeedsLookup reports whether user-position lookup is required.
func UserCatalogPositionNeedsLookup(fields SourceDataPositionFields) bool {
	return !fields.HasTotal || !fields.HasName || !fields.HasUnit
}

func userCatalogCodeKey(code string) string {
	return strings.TrimSpace(extractSourceDataPositionCipher(code))
}
