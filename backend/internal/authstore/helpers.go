package authstore

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"nav-saas-mvp/backend/internal/domain"
)

func normalizeCompanyName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	replacements := []struct{ from, to string }{
		{"«", "\""}, {"»", "\""},
		{"“", "\""}, {"”", "\""},
		{"„", "\""}, {"‟", "\""},
	}
	for _, item := range replacements {
		name = strings.ReplaceAll(name, item.from, item.to)
	}
	return name
}

func normalizePersonName(name string) string {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(name)))
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}

	initStart := len(parts)
	for i := 1; i < len(parts); i++ {
		if looksLikeInitial(parts[i]) {
			initStart = i
			break
		}
	}
	if initStart == len(parts) {
		return strings.Join(parts, " ")
	}

	surname := strings.Join(parts[:initStart], " ")
	initials := compactInitials(parts[initStart:])
	if initials == "" {
		return surname
	}
	return strings.TrimSpace(surname + " " + initials)
}

func looksLikeInitial(part string) bool {
	part = strings.TrimSuffix(part, ".")
	runes := []rune(part)
	return len(runes) > 0 && len(runes) <= 2
}

func compactInitials(parts []string) string {
	letters := make([]rune, 0, len(parts))
	for _, part := range parts {
		for _, r := range part {
			if r == '.' || r == ' ' {
				continue
			}
			letters = append(letters, r)
		}
	}
	if len(letters) == 0 {
		return ""
	}

	var b strings.Builder
	for i, r := range letters {
		if i > 0 {
			b.WriteRune('.')
		}
		b.WriteRune(r)
	}
	b.WriteRune('.')
	return b.String()
}

func newID(prefix string) string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return prefix + "_" + hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return prefix + "_" + hex.EncodeToString(bytes)
}

func normalizeLicenseItems(items map[string]int) map[string]int {
	normalized := make(map[string]int, len(domain.BaseSubsections()))
	for _, subsection := range domain.BaseSubsections() {
		count := 0
		if items != nil {
			if value, ok := items[string(subsection.ID)]; ok && value > 0 {
				count = value
			}
		}
		normalized[string(subsection.ID)] = count
	}
	return normalized
}
