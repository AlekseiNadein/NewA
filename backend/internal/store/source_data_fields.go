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
