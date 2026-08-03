package store

import (
	"strings"
	"unicode"
)

const sourceDataFieldDelimiter = "'"

// SourceDataResourceReplacement is a (Р<code1>Р<code2>) correction: replace resource 1 with resource 2.
// FromNumber/ToNumber are digit-only keys (labels like М/С are stripped).
type SourceDataResourceReplacement struct {
	FromNumber string
	ToNumber   string
}

// SourceDataResourceDeletion is a (Р<code>) correction: remove that resource from the norm.
// Number is the digit-only key (labels like М/С are stripped).
type SourceDataResourceDeletion struct {
	Number string
}

// SplitSourceDataFields splits a source-data record line into fields.
// Apostrophes are field delimiters only; there is no escaping.
func SplitSourceDataFields(line string) []string {
	return strings.Split(strings.TrimSpace(line), sourceDataFieldDelimiter)
}

func sourceDataLineFields(rawText string) []string {
	rawText = strings.TrimSpace(rawText)
	if rawText == "" || strings.HasPrefix(rawText, "ПР") || strings.HasPrefix(rawText, "Р") {
		return nil
	}
	fields := SplitSourceDataFields(rawText)
	if len(fields) < 2 {
		return nil
	}
	return fields
}

// ExtractSourceDataDeterminantAssignment returns the value of a (=...) correction
// after the cipher. Example: "ТПрайс-лист(=14)" → "14".
// Other parenthetical groups such as "(РМ...)" or "(KLink=...)" are ignored.
func ExtractSourceDataDeterminantAssignment(firstField string) string {
	raw := strings.TrimSpace(firstField)
	if raw == "" {
		return ""
	}
	searchFrom := 0
	for {
		open := strings.Index(raw[searchFrom:], "(=")
		if open < 0 {
			return ""
		}
		open += searchFrom
		close := strings.Index(raw[open+2:], ")")
		if close < 0 {
			return ""
		}
		close += open + 2
		return strings.TrimSpace(raw[open+2 : close])
	}
}

// SourceDataDeterminantFromRawText reads (=...) from the first field of a source-data line.
func SourceDataDeterminantFromRawText(rawText string) string {
	fields := sourceDataLineFields(rawText)
	if fields == nil {
		return ""
	}
	return ExtractSourceDataDeterminantAssignment(fields[0])
}

// ExtractSourceDataResourceReplacements returns (РfromРto) replacements from the cipher field.
// Example: "Е0801-002-02 (РМ11762РМ6141)" → [{From:11762, To:6141}].
// Single-token groups like "(РМ34239)" are deletions (see ExtractSourceDataResourceDeletions).
// "(KLink=...)" and "(=N)" are ignored. An optional "=factor" after the second code is ignored.
func ExtractSourceDataResourceReplacements(firstField string) []SourceDataResourceReplacement {
	raw := strings.TrimSpace(firstField)
	if raw == "" {
		return nil
	}
	out := make([]SourceDataResourceReplacement, 0)
	for _, group := range extractSourceDataParentheticalGroups(raw) {
		from, to, ok := parseResourceReplacementGroup(group)
		if !ok {
			continue
		}
		out = append(out, SourceDataResourceReplacement{
			FromNumber: from,
			ToNumber:   to,
		})
	}
	return out
}

// SourceDataResourceReplacementsFromRawText reads (Р…Р…) from the first field of a source-data line.
func SourceDataResourceReplacementsFromRawText(rawText string) []SourceDataResourceReplacement {
	fields := sourceDataLineFields(rawText)
	if fields == nil {
		return nil
	}
	return ExtractSourceDataResourceReplacements(fields[0])
}

// ExtractSourceDataResourceDeletions returns (Р<code>) deletions from the cipher field.
// Example: "Е0624-003-02 (РМ34239)" → [{Number:34239}].
// Two-token replacement groups, "(KLink=...)", and "(=N)" are ignored.
// An optional "=factor" suffix after the code is ignored for matching.
func ExtractSourceDataResourceDeletions(firstField string) []SourceDataResourceDeletion {
	raw := strings.TrimSpace(firstField)
	if raw == "" {
		return nil
	}
	out := make([]SourceDataResourceDeletion, 0)
	for _, group := range extractSourceDataParentheticalGroups(raw) {
		number, ok := parseResourceDeletionGroup(group)
		if !ok {
			continue
		}
		out = append(out, SourceDataResourceDeletion{Number: number})
	}
	return out
}

// SourceDataResourceDeletionsFromRawText reads (Р…) deletions from the first field of a source-data line.
func SourceDataResourceDeletionsFromRawText(rawText string) []SourceDataResourceDeletion {
	fields := sourceDataLineFields(rawText)
	if fields == nil {
		return nil
	}
	return ExtractSourceDataResourceDeletions(fields[0])
}

func extractSourceDataParentheticalGroups(raw string) []string {
	groups := make([]string, 0)
	searchFrom := 0
	for {
		open := strings.IndexByte(raw[searchFrom:], '(')
		if open < 0 {
			return groups
		}
		open += searchFrom
		close := strings.IndexByte(raw[open+1:], ')')
		if close < 0 {
			return groups
		}
		close += open + 1
		groups = append(groups, strings.TrimSpace(raw[open+1:close]))
		searchFrom = close + 1
	}
}

func parseResourceReplacementGroup(inner string) (fromNumber, toNumber string, ok bool) {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return "", "", false
	}
	if strings.HasPrefix(inner, "=") || strings.HasPrefix(strings.ToLower(inner), "klink=") {
		return "", "", false
	}

	fromNumber, rest, ok := readResourceReplacementToken(inner)
	if !ok {
		return "", "", false
	}
	toNumber, rest, ok = readResourceReplacementToken(rest)
	if !ok {
		return "", "", false
	}
	rest = strings.TrimSpace(rest)
	if rest != "" && !strings.HasPrefix(rest, "=") {
		return "", "", false
	}
	if fromNumber == "" || toNumber == "" {
		return "", "", false
	}
	return fromNumber, toNumber, true
}

// parseResourceDeletionGroup accepts exactly one Р+digits token (optional "=factor" suffix).
func parseResourceDeletionGroup(inner string) (number string, ok bool) {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return "", false
	}
	if strings.HasPrefix(inner, "=") || strings.HasPrefix(strings.ToLower(inner), "klink=") {
		return "", false
	}

	number, rest, ok := readResourceReplacementToken(inner)
	if !ok || number == "" {
		return "", false
	}
	// Two tokens ⇒ replacement, not deletion.
	if _, _, secondOK := readResourceReplacementToken(rest); secondOK {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	if rest != "" && !strings.HasPrefix(rest, "=") {
		return "", false
	}
	return number, true
}

// readResourceReplacementToken parses "Р" + optional type letter + digits.
// Accepts Cyrillic Р and Latin P; type letters М/С/Т (and Latin equivalents).
func readResourceReplacementToken(raw string) (number, rest string, ok bool) {
	runes := []rune(strings.TrimSpace(raw))
	if len(runes) < 2 {
		return "", raw, false
	}
	if runes[0] != 'Р' && runes[0] != 'P' {
		return "", raw, false
	}
	i := 1
	if i < len(runes) && isResourceTypeLetter(runes[i]) {
		i++
	}
	start := i
	for i < len(runes) && unicode.IsDigit(runes[i]) {
		i++
	}
	if i == start {
		return "", raw, false
	}
	return string(runes[start:i]), string(runes[i:]), true
}

func isResourceTypeLetter(r rune) bool {
	switch r {
	case 'М', 'M', 'С', 'C', 'Т', 'T':
		return true
	default:
		return false
	}
}

func quantityRawFromSourceDataLine(rawText string) string {
	fields := sourceDataLineFields(rawText)
	if fields == nil {
		return ""
	}
	return strings.TrimSpace(fields[1])
}

func nameFromSourceDataLine(rawText string) string {
	fields := sourceDataLineFields(rawText)
	if fields == nil || len(fields) < 4 {
		return ""
	}
	return strings.TrimSpace(fields[3])
}

func unitFromSourceDataLine(rawText string) string {
	fields := sourceDataLineFields(rawText)
	if fields == nil || len(fields) < 5 {
		return ""
	}
	return strings.TrimSpace(fields[4])
}

func quantityFromSourceDataLine(rawText string) (float64, error) {
	return ParseSourceDataQuantity(quantityRawFromSourceDataLine(rawText))
}

// ResolveEstimateLineQuantity returns the line volume used for pricing.
// Stored quantity takes precedence; otherwise the second field of rawText is parsed.
func ResolveEstimateLineQuantity(stored float64, rawText string) (float64, error) {
	if stored > 0 {
		return stored, nil
	}
	return quantityFromSourceDataLine(rawText)
}
