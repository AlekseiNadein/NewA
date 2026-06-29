package store

import "strings"

const sourceDataFieldDelimiter = "'"

// SplitSourceDataFields splits a source-data record line into fields.
// Apostrophes are field delimiters only; there is no escaping.
func SplitSourceDataFields(line string) []string {
	return strings.Split(strings.TrimSpace(line), sourceDataFieldDelimiter)
}

func quantityRawFromSourceDataLine(rawText string) string {
	rawText = strings.TrimSpace(rawText)
	if rawText == "" || strings.HasPrefix(rawText, "ПР") || strings.HasPrefix(rawText, "Р") {
		return ""
	}
	fields := SplitSourceDataFields(rawText)
	if len(fields) < 2 {
		return ""
	}
	return strings.TrimSpace(fields[1])
}

func quantityFromSourceDataLine(rawText string) (float64, error) {
	return ParseSourceDataQuantity(quantityRawFromSourceDataLine(rawText))
}
