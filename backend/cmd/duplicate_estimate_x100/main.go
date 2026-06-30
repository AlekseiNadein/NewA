package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const multiply = 100

var sourceTitles = []string{
	"Архитектурные решения_АР",
	"Конструктивные решения_КР",
}

type estimateRow struct {
	ID          string
	CompanyID   string
	ObjectID    string
	Code        string
	Title       string
	Description string
	District    string
	FgisSetID   string
	Status      string
}

type lineRow struct {
	LineType     string
	Source       string
	Code         string
	OriginalCode string
	Name         string
	Quantity     float64
	Unit         string
	UnitPrice    float64
	Total        float64
	RawText      string
	ParsedJSON   []byte
	CalcJSON     []byte
	CalcStatus   string
	CalcError    string
	Revision     int64
	CalculatedAt *time.Time
}

func main() {
	databaseURL := os.Getenv("APP_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "user=postgres password=postgres dbname=postgres sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		fatal("connect:", err)
	}
	defer pool.Close()

	for _, title := range sourceTitles {
		if err := duplicateEstimate(ctx, pool, title); err != nil {
			fatal(title+":", err)
		}
	}
}

func duplicateEstimate(ctx context.Context, pool *pgxpool.Pool, sourceTitle string) error {
	newTitle := "X100 " + sourceTitle

	if _, err := pool.Exec(ctx, `DELETE FROM app_estimates WHERE title = $1`, newTitle); err != nil {
		return err
	}

	var est estimateRow
	err := pool.QueryRow(ctx, `
SELECT id, company_id, object_id, code, title, description, district, fgis_set_id, status
FROM app_estimates WHERE title = $1
ORDER BY updated_at DESC LIMIT 1`, sourceTitle).Scan(
		&est.ID, &est.CompanyID, &est.ObjectID, &est.Code, &est.Title,
		&est.Description, &est.District, &est.FgisSetID, &est.Status,
	)
	if err != nil {
		return fmt.Errorf("source %q not found: %w", sourceTitle, err)
	}

	lineRows, err := pool.Query(ctx, `
SELECT line_type, source, code, original_code, name, quantity, unit, unit_price, total,
       raw_text, parsed_json, calc_json, calc_status, calc_error, revision, calculated_at
FROM app_estimate_lines
WHERE estimate_id = $1
ORDER BY sort_order, id`, est.ID)
	if err != nil {
		return err
	}
	defer lineRows.Close()

	var lines []lineRow
	for lineRows.Next() {
		var line lineRow
		if err := lineRows.Scan(
			&line.LineType, &line.Source, &line.Code, &line.OriginalCode, &line.Name,
			&line.Quantity, &line.Unit, &line.UnitPrice, &line.Total, &line.RawText,
			&line.ParsedJSON, &line.CalcJSON, &line.CalcStatus, &line.CalcError,
			&line.Revision, &line.CalculatedAt,
		); err != nil {
			return err
		}
		lines = append(lines, line)
	}
	if err := lineRows.Err(); err != nil {
		return err
	}

	newEstimateID := newID("est")
	newCode := "X100-" + est.Code
	now := time.Now().UTC()

	var blockTotal float64
	positionCount := 0
	for _, line := range lines {
		if line.LineType == "position" {
			positionCount++
			blockTotal += line.Total
		}
	}

	var expanded []lineRow
	for range multiply {
		expanded = append(expanded, lines...)
	}
	total := blockTotal * multiply

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
INSERT INTO app_estimates (id, company_id, object_id, code, title, description, district, fgis_set_id, status, total, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		newEstimateID, est.CompanyID, est.ObjectID, newCode, newTitle,
		est.Description, est.District, est.FgisSetID, est.Status, total, now, now,
	)
	if err != nil {
		return err
	}

	for i, line := range expanded {
		lineID := newID("itm")
		_, err = tx.Exec(ctx, `
INSERT INTO app_estimate_lines (
  id, estimate_id, line_type, source, code, original_code, name, quantity, unit, unit_price, total,
  raw_text, parsed_json, calc_json, calc_status, calc_error, revision, calculated_at, sort_order
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
			lineID, newEstimateID, line.LineType, line.Source, line.Code, line.OriginalCode, line.Name,
			line.Quantity, line.Unit, line.UnitPrice, line.Total, line.RawText,
			nullJSON(line.ParsedJSON), nullJSON(line.CalcJSON),
			line.CalcStatus, line.CalcError, line.Revision, line.CalculatedAt, i,
		)
		if err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	fmt.Printf("created %q (%s): %d lines (%d source lines x%d blocks), %d positions, total %.2f\n",
		newTitle, newEstimateID, len(expanded), len(lines), multiply, positionCount*multiply, total)
	return nil
}

func nullJSON(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func newID(prefix string) string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return prefix + "_" + hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return prefix + "_" + hex.EncodeToString(bytes)
}

func fatal(parts ...any) {
	msg := strings.TrimSpace(fmt.Sprint(parts...))
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
