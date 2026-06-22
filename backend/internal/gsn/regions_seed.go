package gsn

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

type RegionSeedRow struct {
	Code    string
	Name    string
	LineNo  int
	RawLine string
}

func ParseRegionsFile(path string) ([]RegionSeedRow, error) {
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
	rows := make([]RegionSeedRow, 0)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		rawLine := scanner.Text()
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" {
			continue
		}

		parts := strings.SplitN(trimmed, " ", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("line %d: expected \"<code> <name>\", got %q", lineNo, trimmed)
		}

		code := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		if code == "" || name == "" {
			return nil, fmt.Errorf("line %d: empty code or name in %q", lineNo, trimmed)
		}

		rows = append(rows, RegionSeedRow{
			Code:    code,
			Name:    name,
			LineNo:  lineNo,
			RawLine: rawLine,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return rows, nil
}
