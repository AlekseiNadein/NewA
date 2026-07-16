package gsn

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

type ResourceCodifierRow struct {
	Number string
	Code   string
	Name   string
	Unit   string
	LineNo int
}

func splitGSNRecordLine(line string) []string {
	body := strings.TrimRight(line, "\r\n")
	if strings.HasSuffix(body, "*") {
		body = body[:len(body)-1]
	}
	return strings.Split(body, "'")
}

func isAllDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// extractNormCodeFromCodifierField extracts the norm cipher from field 2 of 00~resurs.txt (####шифр нормы).
func extractNormCodeFromCodifierField(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	for _, part := range strings.Split(raw, "#") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		runes := []rune(part)
		if len(runes) < 2 {
			continue
		}
		switch runes[0] {
		case 'С', 'C', 'М', 'M', 'Т', 'T':
			return part
		}
	}
	return ""
}

func resourceNumberKey(resourceCode string) string {
	resourceCode = strings.TrimSpace(resourceCode)
	if resourceCode == "" {
		return ""
	}

	runes := []rune(resourceCode)
	if len(runes) >= 2 {
		switch runes[0] {
		case 'С', 'C', 'М', 'M', 'Т', 'T':
			if unicode.IsDigit(runes[1]) {
				resourceCode = string(runes[1:])
			}
		}
	}

	var digits strings.Builder
	for _, r := range resourceCode {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	return digits.String()
}

func ParseResourceCodifierFile(path string) ([]ResourceCodifierRow, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(content) {
		decoded, err := charmap.Windows1251.NewDecoder().Bytes(content)
		if err != nil {
			return nil, fmt.Errorf("decode cp1251: %w", err)
		}
		content = decoded
	}

	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	rows := make([]ResourceCodifierRow, 0)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		fields := splitGSNRecordLine(scanner.Text())
		if len(fields) == 0 {
			continue
		}

		number := strings.TrimSpace(fields[0])
		if !isAllDigits(number) {
			continue
		}

		var codeRaw string
		if len(fields) > 2 {
			codeRaw = fields[2]
		}
		name := ""
		if len(fields) > 3 {
			name = strings.TrimSpace(fields[3])
		}
		unit := ""
		if len(fields) > 4 {
			unit = strings.TrimSpace(fields[4])
		}

		rows = append(rows, ResourceCodifierRow{
			Number: number,
			Code:   extractNormCodeFromCodifierField(codeRaw),
			Name:   name,
			Unit:   unit,
			LineNo: lineNo,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Service) lookupResourceCodifier(ctx context.Context, number string) (ResourceCodifierRow, bool, error) {
	rows, err := s.lookupResourceCodifiers(ctx, []string{number})
	if err != nil {
		return ResourceCodifierRow{}, false, err
	}
	row, ok := rows[strings.TrimSpace(number)]
	return row, ok, nil
}

func (s *Service) lookupResourceCodifiers(ctx context.Context, numbers []string) (map[string]ResourceCodifierRow, error) {
	unique := uniqueNonEmptyStrings(numbers)
	if len(unique) == 0 {
		return map[string]ResourceCodifierRow{}, nil
	}

	inClause, args := sqlInClause(1, unique)
	query := fmt.Sprintf(`
		SELECT number, code, name, unit
		FROM gsn.resource_codifier
		WHERE number IN (%s)
	`, inClause)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query resource codifiers: %w", err)
	}
	defer rows.Close()

	items := make(map[string]ResourceCodifierRow, len(unique))
	for rows.Next() {
		var row ResourceCodifierRow
		if err := rows.Scan(&row.Number, &row.Code, &row.Name, &row.Unit); err != nil {
			return nil, fmt.Errorf("scan resource codifier: %w", err)
		}
		items[row.Number] = row
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read resource codifiers: %w", err)
	}
	return items, nil
}
