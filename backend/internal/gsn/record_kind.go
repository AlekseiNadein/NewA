package gsn

import (
	"strings"
	"unicode"
)

func isWorkNormRecord(code, recordKind string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(recordKind)) {
	case "resource_catalog", "coefficient_catalog":
		return false
	}

	runes := []rune(code)
	if len(runes) == 0 {
		return false
	}

	switch runes[0] {
	case 'Е', 'У', 'E', 'U':
		return true
	case 'С', 'C', 'М', 'M', 'Т', 'T':
		return false
	case 'Ц':
		return strings.EqualFold(recordKind, "norm")
	}
	return strings.EqualFold(recordKind, "norm")
}

func isResourcePositionCode(code string) bool {
	code = strings.TrimSpace(code)
	runes := []rune(code)
	if len(runes) < 2 {
		return false
	}
	switch runes[0] {
	case 'С', 'C', 'М', 'M', 'Т', 'T':
		return unicode.IsDigit(runes[1])
	}
	return false
}
