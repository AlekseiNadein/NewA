package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"nav-saas-mvp/backend/internal/domain"
)

// EstimateCalcBatchResponse is a client-facing batch of calculated line statuses.
type EstimateCalcBatchResponse struct {
	Generation int64                `json:"generation"`
	Applied    int                    `json:"applied"`
	Total      int                    `json:"total"`
	Processed  int                    `json:"processed"`
	Errors     int                    `json:"errors"`
	GrandTotal float64              `json:"grandTotal"`
	Done       bool                 `json:"done"`
	Reset      bool                 `json:"reset"`
	Items      []EstimateCalcStatus `json:"items"`
}

func isCalcTerminalStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "done", "failed", "dead":
		return true
	default:
		return false
	}
}

func (s *FileStore) loadEstimateCalcMeta(ctx context.Context, companyID, estimateID string, includeAll bool) (domain.Estimate, error) {
	if s.treeDB == nil {
		return domain.Estimate{}, nil
	}
	query := `
SELECT company_id, district, fgis_set_id, calc_generation
FROM app_estimates
WHERE id = $1`
	args := []any{estimateID}
	if !includeAll {
		query += ` AND company_id = $2`
		args = append(args, companyID)
	}
	var estimate domain.Estimate
	estimate.ID = estimateID
	err := s.treeDB.QueryRow(ctx, query, args...).Scan(
		&estimate.CompanyID,
		&estimate.District,
		&estimate.FgisSetID,
		&estimate.CalcGeneration,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Estimate{}, ErrNotFound
		}
		return domain.Estimate{}, err
	}
	if !includeAll && estimate.CompanyID != companyID {
		return domain.Estimate{}, ErrForbidden
	}
	return estimate, nil
}

// CancelEstimateCalc invalidates in-flight calc jobs for an estimate and bumps calc_generation.
func (s *FileStore) CancelEstimateCalc(ctx context.Context, companyID, estimateID string, includeAll bool) (int64, error) {
	if s.treeDB == nil {
		return 0, nil
	}

	tx, err := s.treeDB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	query := `
UPDATE app_estimates
SET calc_generation = calc_generation + 1
WHERE id = $1`
	args := []any{estimateID}
	if !includeAll {
		query += ` AND company_id = $2`
		args = append(args, companyID)
	}
	query += ` RETURNING calc_generation, company_id, district, fgis_set_id`

	var estimate domain.Estimate
	estimate.ID = estimateID
	err = tx.QueryRow(ctx, query, args...).Scan(
		&estimate.CalcGeneration,
		&estimate.CompanyID,
		&estimate.District,
		&estimate.FgisSetID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}

	rows, err := tx.Query(ctx, `
SELECT id, line_type, source, code, original_code, name, quantity, unit, unit_price, total, raw_text, calc_status
FROM app_estimate_lines
WHERE estimate_id = $1
ORDER BY sort_order, id
`, estimateID)
	if err != nil {
		return 0, err
	}

	type lineReset struct {
		id       string
		revision int64
	}
	resets := make([]lineReset, 0)
	for rows.Next() {
		var line domain.EstimateItem
		if err := rows.Scan(
			&line.ID, &line.Type, &line.Source, &line.Code, &line.OriginalCode, &line.Name,
			&line.Quantity, &line.Unit, &line.UnitPrice, &line.Total, &line.RawText, &line.CalcStatus,
		); err != nil {
			rows.Close()
			return 0, err
		}
		if !shouldEnqueueEstimateLineCalc(line) {
			continue
		}
		resets = append(resets, lineReset{
			id:       line.ID,
			revision: estimateLineRevision(estimate, line),
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	for _, reset := range resets {
		if _, err := tx.Exec(ctx, `
UPDATE app_estimate_lines
SET revision = $3,
    calc_status = '',
    calc_error = '',
    calc_json = NULL,
    unit_price = 0,
    total = 0,
    calculated_at = NULL
WHERE estimate_id = $1 AND id = $2
`, estimateID, reset.id, reset.revision); err != nil {
			return 0, err
		}
	}

	if _, err := tx.Exec(ctx, `
DELETE FROM outbox_events
WHERE published_at IS NULL
  AND payload->>'estimateId' = $1
`, estimateID); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx, `
UPDATE estimate_calc_jobs
SET status = 'dead',
    last_error = 'cancelled',
    leased_until = NULL,
    locked_by = '',
    updated_at = now()
WHERE estimate_id = $1
  AND status IN ('queued', 'leased')
`, estimateID); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	s.unmarkEstimateCalcStartJob(estimateID)
	return estimate.CalcGeneration, nil
}

func (s *FileStore) listEstimateCalcLineStatuses(ctx context.Context, estimateID string) ([]EstimateCalcStatus, error) {
	rows, err := s.treeDB.Query(ctx, `
SELECT l.id, l.revision, l.calc_status, l.calc_error, l.code, l.original_code, l.name, l.unit, l.quantity, l.unit_price, l.total, l.calc_json, l.calculated_at
FROM app_estimate_lines l
WHERE l.estimate_id = $1
  AND l.line_type = $2
  AND l.source = 'gsn'
  AND COALESCE(NULLIF(TRIM(l.code), ''), NULLIF(TRIM(l.original_code), '')) IS NOT NULL
ORDER BY l.sort_order, l.id
`, estimateID, string(domain.EstimateLinePosition))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]EstimateCalcStatus, 0)
	for rows.Next() {
		var item EstimateCalcStatus
		if err := rows.Scan(
			&item.LineID, &item.Revision, &item.Status, &item.Error, &item.Code, &item.OriginalCode,
			&item.Name, &item.Unit, &item.Quantity, &item.UnitPrice, &item.Total, &item.CalcJSON, &item.CalculatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func buildEstimateCalcBatchResponse(
	generation int64,
	applied int,
	batchSize int,
	lines []EstimateCalcStatus,
	reset bool,
) (EstimateCalcBatchResponse, bool) {
	resp := EstimateCalcBatchResponse{
		Generation: generation,
		Applied:    applied,
		Items:      []EstimateCalcStatus{},
		Reset:      reset,
	}
	if batchSize <= 0 {
		batchSize = defaultAppSettings().CalcClientBatchSize
	}

	terminal := make([]EstimateCalcStatus, 0, len(lines))
	processed := 0
	errorsCount := 0
	grandTotal := 0.0
	for _, line := range lines {
		status := strings.TrimSpace(line.Status)
		if isCalcTerminalStatus(status) {
			terminal = append(terminal, line)
			processed++
			if status == "failed" || status == "dead" {
				errorsCount++
			}
			if status == "done" {
				grandTotal += line.Total
			}
		}
	}

	resp.Total = len(lines)
	resp.Processed = processed
	resp.Errors = errorsCount
	resp.GrandTotal = grandTotal
	resp.Done = resp.Total > 0 && processed >= resp.Total

	if applied < 0 {
		applied = 0
	}
	if applied > len(terminal) {
		applied = len(terminal)
	}

	newTerminal := len(terminal) - applied
	ready := newTerminal >= batchSize || (resp.Done && newTerminal > 0) || (resp.Done && newTerminal == 0 && applied > 0)
	if !ready {
		return resp, false
	}

	count := batchSize
	if newTerminal < count {
		count = newTerminal
	}
	end := applied + count
	if end > len(terminal) {
		end = len(terminal)
	}

	if end > applied {
		resp.Items = append(resp.Items, terminal[applied:end]...)
	}
	resp.Applied = applied + len(resp.Items)
	return resp, true
}

func (s *FileStore) fetchEstimateCalcBatchOnce(
	ctx context.Context,
	companyID, estimateID string,
	includeAll bool,
	applied int,
	clientGeneration int64,
	batchSize int,
) (EstimateCalcBatchResponse, bool, error) {
	estimate, err := s.loadEstimateCalcMeta(ctx, companyID, estimateID, includeAll)
	if err != nil {
		return EstimateCalcBatchResponse{}, false, err
	}
	if clientGeneration > 0 && clientGeneration != estimate.CalcGeneration {
		resp, ready := buildEstimateCalcBatchResponse(estimate.CalcGeneration, 0, batchSize, nil, true)
		return resp, ready, nil
	}

	lines, err := s.listEstimateCalcLineStatuses(ctx, estimateID)
	if err != nil {
		return EstimateCalcBatchResponse{}, false, err
	}
	resp, ready := buildEstimateCalcBatchResponse(estimate.CalcGeneration, applied, batchSize, lines, false)
	return resp, ready, nil
}

// FetchEstimateCalcBatch returns the next batch of terminal calc statuses for the client.
func (s *FileStore) FetchEstimateCalcBatch(
	ctx context.Context,
	companyID, estimateID string,
	includeAll bool,
	applied int,
	clientGeneration int64,
	wait bool,
	timeout time.Duration,
) (EstimateCalcBatchResponse, error) {
	if s.treeDB == nil {
		return EstimateCalcBatchResponse{}, nil
	}
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	settings, err := s.GetAppSettings()
	if err != nil {
		return EstimateCalcBatchResponse{}, err
	}
	batchSize := settings.CalcClientBatchSize

	deadline := time.Now().Add(timeout)
	for {
		resp, ready, err := s.fetchEstimateCalcBatchOnce(ctx, companyID, estimateID, includeAll, applied, clientGeneration, batchSize)
		if err != nil {
			return EstimateCalcBatchResponse{}, err
		}
		if ready || !wait || time.Now().After(deadline) {
			return resp, nil
		}
		select {
		case <-ctx.Done():
			return EstimateCalcBatchResponse{}, ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
}
