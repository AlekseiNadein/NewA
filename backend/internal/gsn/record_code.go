package gsn

import (
	"fmt"
	"strconv"
	"strings"
)

// NormalizeResourceCatalogCode pads numeric segments of catalog resource codes to the
// canonical GSN form (e.g. С1111-301-62 → С1111-0301-0062). Norms (Е/Ц) are unchanged.
func NormalizeResourceCatalogCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" || !isResourcePositionCode(code) {
		return code
	}

	parts := strings.Split(code, "-")
	if len(parts) < 3 {
		return code
	}

	out := make([]string, 0, len(parts))
	out = append(out, parts[0])
	for _, part := range parts[1:] {
		out = append(out, padRecordCodeSegment(part))
	}
	return strings.Join(out, "-")
}

func padRecordCodeSegment(part string) string {
	part = strings.TrimSpace(part)
	if part == "" {
		return part
	}

	value, err := strconv.Atoi(part)
	if err != nil {
		return part
	}
	return fmt.Sprintf("%04d", value)
}

func recordLookupCandidates(code string) []string {
	code = ExtractPositionCipher(code)
	seen := map[string]struct{}{}
	out := make([]string, 0, 2)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}

	add(code)
	add(NormalizeResourceCatalogCode(code))
	return out
}
