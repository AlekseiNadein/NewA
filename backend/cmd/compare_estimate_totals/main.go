package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "user=postgres password=postgres dbname=postgres sslmode=disable")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer pool.Close()

	estimateID := "est_1b0e1363326ce2ab"
	rows, err := pool.Query(ctx, `
SELECT total, raw_text, calc_status
FROM app_estimate_lines
WHERE estimate_id = $1
  AND line_type = 'position'
  AND source = 'gsn'
  AND COALESCE(NULLIF(TRIM(code), ''), NULLIF(TRIM(original_code), '')) IS NOT NULL
`, estimateID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer rows.Close()

	var dbDone, rawDone float64
	var doneCount, withRawTotal int
	for rows.Next() {
		var dbTotal float64
		var rawText, status string
		if err := rows.Scan(&dbTotal, &rawText, &status); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		rawTotal := 0.0
		hasRaw := false
		if fields, err := parseSourceDataPositionFields(rawText); err == nil && fields.hasTotal {
			rawTotal = fields.total
			hasRaw = true
		}
		if status == "done" {
			doneCount++
			dbDone += dbTotal
			if hasRaw {
				withRawTotal++
				rawDone += rawTotal
			}
		}
	}
	if err := rows.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("done lines: %d (with raw total: %d)\n", doneCount, withRawTotal)
	fmt.Printf("DB done sum:     %.2f\n", dbDone)
	fmt.Printf("Raw done sum:    %.2f\n", rawDone)
	fmt.Printf("Difference:      %.2f\n", dbDone-rawDone)
}

type sourceFields struct {
	hasTotal bool
	total    float64
}

func parseSourceDataPositionFields(rawText string) (sourceFields, error) {
	fields := sourceDataLineFields(rawText)
	if fields == nil {
		return sourceFields{}, fmt.Errorf("invalid line")
	}
	out := sourceFields{}
	totalIdx := 2
	if len(fields) > 2 && isIndexMarker(fields[2]) {
		totalIdx = 3
	}
	if totalIdx < len(fields) {
		raw := strings.TrimSpace(fields[totalIdx])
		if raw != "" {
			n := strings.ReplaceAll(strings.ReplaceAll(raw, " ", ""), ",", ".")
			v, err := strconv.ParseFloat(n, 64)
			if err != nil {
				return sourceFields{}, err
			}
			out.hasTotal = true
			out.total = v
		}
	}
	return out, nil
}

func isIndexMarker(raw string) bool {
	raw = strings.TrimSpace(raw)
	return len(raw) >= 3 && raw[0] == '[' && raw[len(raw)-1] == ']'
}

func sourceDataLineFields(rawText string) []string {
	line := strings.TrimSpace(rawText)
	if line == "" || strings.HasPrefix(line, "Р") || strings.HasPrefix(line, "ПР") {
		return nil
	}
	return strings.Split(line, "'")
}
