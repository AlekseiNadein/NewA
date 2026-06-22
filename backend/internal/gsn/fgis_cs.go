package gsn

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

type FGISPriceIndexRow struct {
	Code    string
	Value   string
	LineNo  int
	RawLine string
}

type FGISSetRow struct {
	Code        string
	Prices      string
	Indexes     string
	LineNoPrice int
	LineNoIndex int
}

const fgisCSSchemaSQL = `
CREATE SCHEMA IF NOT EXISTS fgis_cs;

CREATE TABLE IF NOT EXISTS fgis_cs.sets (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    prices_source TEXT NOT NULL DEFAULT '',
    indexes_source TEXT NOT NULL DEFAULT '',
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fgis_cs.set_rows (
    set_id TEXT NOT NULL REFERENCES fgis_cs.sets(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    prices TEXT NOT NULL DEFAULT '',
    indexes TEXT NOT NULL DEFAULT '',
    line_no_prices INTEGER,
    line_no_indexes INTEGER,
    PRIMARY KEY (set_id, code)
);

CREATE INDEX IF NOT EXISTS idx_fgis_cs_set_rows_code ON fgis_cs.set_rows(code);
`

func readCP1251File(path string) ([]byte, error) {
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
	return content, nil
}

func isFGISHeaderCode(code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return true
	}
	if strings.HasSuffix(code, "-0") && len([]rune(code)) <= 4 {
		return true
	}
	return false
}

func parseFGISLine(rawLine string) (code, value string, ok bool) {
	body := strings.TrimSpace(rawLine)
	if body == "" {
		return "", "", false
	}
	if strings.HasSuffix(body, "*") {
		body = body[:len(body)-1]
	}

	parts := strings.Split(body, "'")
	if len(parts) < 2 {
		return "", "", false
	}

	code = strings.TrimSpace(parts[0])
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part != "" {
			value = part
			break
		}
	}
	if code == "" || value == "" || isFGISHeaderCode(code) {
		return "", "", false
	}

	return code, value, true
}

func ParseFGISPriceIndexFile(path string) ([]FGISPriceIndexRow, error) {
	content, err := readCP1251File(path)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	rows := make([]FGISPriceIndexRow, 0)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		rawLine := scanner.Text()
		code, value, ok := parseFGISLine(rawLine)
		if !ok {
			continue
		}

		rows = append(rows, FGISPriceIndexRow{
			Code:    code,
			Value:   value,
			LineNo:  lineNo,
			RawLine: rawLine,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return rows, nil
}

func MergeFGISSetRows(pricesPath, indexesPath string) ([]FGISSetRow, error) {
	priceRows, err := ParseFGISPriceIndexFile(pricesPath)
	if err != nil {
		return nil, fmt.Errorf("parse prices: %w", err)
	}
	indexRows, err := ParseFGISPriceIndexFile(indexesPath)
	if err != nil {
		return nil, fmt.Errorf("parse indexes: %w", err)
	}

	merged := make(map[string]*FGISSetRow, len(priceRows)+len(indexRows))
	order := make([]string, 0, len(priceRows)+len(indexRows))

	addCode := func(code string) *FGISSetRow {
		row, ok := merged[code]
		if ok {
			return row
		}
		row = &FGISSetRow{Code: code}
		merged[code] = row
		order = append(order, code)
		return row
	}

	for _, row := range priceRows {
		target := addCode(row.Code)
		target.Prices = row.Value
		target.LineNoPrice = row.LineNo
	}
	for _, row := range indexRows {
		target := addCode(row.Code)
		target.Indexes = row.Value
		target.LineNoIndex = row.LineNo
	}

	result := make([]FGISSetRow, 0, len(order))
	for _, code := range order {
		result = append(result, *merged[code])
	}

	return result, nil
}

func (s *Service) ImportFGISSet(ctx context.Context, setID, setName, pricesPath, indexesPath string) (int, error) {
	if !s.Configured() {
		return 0, ErrNotConfigured
	}

	setID = strings.TrimSpace(setID)
	setName = strings.TrimSpace(setName)
	if setID == "" {
		return 0, fmt.Errorf("set id is required")
	}
	if setName == "" {
		return 0, fmt.Errorf("set name is required")
	}
	if strings.TrimSpace(pricesPath) == "" {
		return 0, fmt.Errorf("prices source path is required")
	}
	if strings.TrimSpace(indexesPath) == "" {
		return 0, fmt.Errorf("indexes source path is required")
	}

	rows, err := MergeFGISSetRows(pricesPath, indexesPath)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("no rows found in %s and %s", pricesPath, indexesPath)
	}

	if _, err := s.db.ExecContext(ctx, fgisCSSchemaSQL); err != nil {
		return 0, fmt.Errorf("ensure fgis_cs schema: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM fgis_cs.set_rows WHERE set_id = $1`, setID); err != nil {
		return 0, fmt.Errorf("clear fgis_cs.set_rows: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM fgis_cs.sets WHERE id = $1`, setID); err != nil {
		return 0, fmt.Errorf("clear fgis_cs.sets: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO fgis_cs.sets (id, name, prices_source, indexes_source)
		VALUES ($1, $2, $3, $4)
	`, setID, setName, pricesPath, indexesPath); err != nil {
		return 0, fmt.Errorf("insert fgis_cs.sets: %w", err)
	}

	const batchSize = 1000
	for start := 0; start < len(rows); start += batchSize {
		end := start + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[start:end]

		var builder strings.Builder
		args := make([]any, 0, len(batch)*6)
		builder.WriteString(`
			INSERT INTO fgis_cs.set_rows
				(set_id, code, prices, indexes, line_no_prices, line_no_indexes)
			VALUES
		`)

		for i, row := range batch {
			if i > 0 {
				builder.WriteString(",")
			}
			base := len(args) + 1
			fmt.Fprintf(&builder, "($%d,$%d,$%d,$%d,$%d,$%d)", base, base+1, base+2, base+3, base+4, base+5)

			var lineNoPrice any
			if row.LineNoPrice > 0 {
				lineNoPrice = row.LineNoPrice
			}
			var lineNoIndex any
			if row.LineNoIndex > 0 {
				lineNoIndex = row.LineNoIndex
			}

			args = append(args, setID, row.Code, row.Prices, row.Indexes, lineNoPrice, lineNoIndex)
		}

		if _, err := tx.ExecContext(ctx, builder.String(), args...); err != nil {
			return 0, fmt.Errorf("insert fgis_cs.set_rows batch %d: %w", start/batchSize, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return len(rows), nil
}

type FGISSet struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Service) ListFGISSets(ctx context.Context) ([]FGISSet, error) {
	if !s.Configured() {
		return nil, ErrNotConfigured
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name
		FROM fgis_cs.sets
		ORDER BY imported_at DESC, name
	`)
	if err != nil {
		return nil, fmt.Errorf("query fgis_cs sets: %w", err)
	}
	defer rows.Close()

	items := make([]FGISSet, 0)
	for rows.Next() {
		var item FGISSet
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			return nil, fmt.Errorf("scan fgis_cs set: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read fgis_cs sets: %w", err)
	}

	return items, nil
}

type FGISSetStats struct {
	PricesCount  int `json:"pricesCount"`
	IndexesCount int `json:"indexesCount"`
	TotalCount   int `json:"totalCount"`
}

type FGISSetRowView struct {
	Code         string `json:"code"`
	OriginalCode string `json:"originalCode"`
	Name         string `json:"name"`
	Unit         string `json:"unit"`
	Prices       string `json:"prices"`
	Indexes      string `json:"indexes"`
}

type FGISSetRowsResult struct {
	Stats         FGISSetStats     `json:"stats"`
	Rows          []FGISSetRowView `json:"rows"`
	Limit         int              `json:"limit"`
	Offset        int              `json:"offset"`
	FilteredTotal int              `json:"filteredTotal"`
}

func (s *Service) FGISSetStats(ctx context.Context, setID string) (FGISSetStats, error) {
	if !s.Configured() {
		return FGISSetStats{}, ErrNotConfigured
	}
	setID = strings.TrimSpace(setID)
	if setID == "" {
		return FGISSetStats{}, fmt.Errorf("set id is required")
	}

	var stats FGISSetStats
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE prices <> ''),
			COUNT(*) FILTER (WHERE indexes <> ''),
			COUNT(*)
		FROM fgis_cs.set_rows
		WHERE set_id = $1
	`, setID).Scan(&stats.PricesCount, &stats.IndexesCount, &stats.TotalCount)
	if err != nil {
		return FGISSetStats{}, fmt.Errorf("query fgis_cs stats: %w", err)
	}

	return stats, nil
}

func (s *Service) ListFGISSetRows(ctx context.Context, setID, search string, limit, offset int) (FGISSetRowsResult, error) {
	if !s.Configured() {
		return FGISSetRowsResult{}, ErrNotConfigured
	}

	setID = strings.TrimSpace(setID)
	if setID == "" {
		return FGISSetRowsResult{}, fmt.Errorf("set id is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	stats, err := s.FGISSetStats(ctx, setID)
	if err != nil {
		return FGISSetRowsResult{}, err
	}

	search = strings.TrimSpace(search)
	filteredTotal := stats.TotalCount

	countQuery := `SELECT COUNT(*) FROM fgis_cs.set_rows r LEFT JOIN gsn.records rec ON rec.code = r.code WHERE r.set_id = $1`
	countArgs := []any{setID}
	if search != "" {
		countQuery += ` AND (r.code ILIKE $2 OR rec.name ILIKE $2 OR rec.original_code ILIKE $2)`
		countArgs = append(countArgs, "%"+search+"%")
	}
	if err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&filteredTotal); err != nil {
		return FGISSetRowsResult{}, fmt.Errorf("count fgis_cs rows: %w", err)
	}

	listQuery := `
		SELECT
			r.code,
			COALESCE(rec.original_code, ''),
			COALESCE(rec.name, ''),
			COALESCE(rec.unit, ''),
			r.prices,
			r.indexes
		FROM fgis_cs.set_rows r
		LEFT JOIN gsn.records rec ON rec.code = r.code
		WHERE r.set_id = $1
	`
	listArgs := []any{setID}
	if search != "" {
		listQuery += ` AND (r.code ILIKE $2 OR rec.name ILIKE $2 OR rec.original_code ILIKE $2)`
		listArgs = append(listArgs, "%"+search+"%")
	}
	listQuery += fmt.Sprintf(` ORDER BY r.code LIMIT $%d OFFSET $%d`, len(listArgs)+1, len(listArgs)+2)
	listArgs = append(listArgs, limit, offset)

	rows, err := s.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return FGISSetRowsResult{}, fmt.Errorf("query fgis_cs rows: %w", err)
	}
	defer rows.Close()

	items := make([]FGISSetRowView, 0, limit)
	for rows.Next() {
		var item FGISSetRowView
		if err := rows.Scan(
			&item.Code,
			&item.OriginalCode,
			&item.Name,
			&item.Unit,
			&item.Prices,
			&item.Indexes,
		); err != nil {
			return FGISSetRowsResult{}, fmt.Errorf("scan fgis_cs row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return FGISSetRowsResult{}, fmt.Errorf("read fgis_cs rows: %w", err)
	}

	return FGISSetRowsResult{
		Stats:         stats,
		Rows:          items,
		Limit:         limit,
		Offset:        offset,
		FilteredTotal: filteredTotal,
	}, nil
}
