package store

import "strings"

const sourceDataFieldDelimiter = "'"

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
