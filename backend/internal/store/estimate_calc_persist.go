package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"nav-saas-mvp/backend/internal/domain"
	"nav-saas-mvp/backend/internal/estimatecalc"
)

const estimateCalcTablesSQL = `
CREATE TABLE IF NOT EXISTS estimate_calc_state (
    estimate_id TEXT PRIMARY KEY REFERENCES app_estimates(id) ON DELETE CASCADE,
    generation BIGINT NOT NULL DEFAULT 0,
    district TEXT NOT NULL DEFAULT '',
    fgis_set_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '' CHECK (status IN ('', 'starting', 'running', 'done', 'failed')),
    grand_total NUMERIC(18, 2) NOT NULL DEFAULT 0,
    lines_total INTEGER NOT NULL DEFAULT 0,
    lines_done INTEGER NOT NULL DEFAULT 0,
    lines_errors INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS estimate_calc_lines (
    estimate_id TEXT NOT NULL REFERENCES app_estimates(id) ON DELETE CASCADE,
    generation BIGINT NOT NULL,
    line_id TEXT NOT NULL,
    line_revision BIGINT NOT NULL,
    position_no INTEGER NOT NULL DEFAULT 0,
    code TEXT NOT NULL DEFAULT '',
    original_code TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL DEFAULT '',
    unit TEXT NOT NULL DEFAULT '',
    quantity NUMERIC(18, 6) NOT NULL DEFAULT 0,
    unit_price NUMERIC(18, 2) NOT NULL DEFAULT 0,
    total NUMERIC(18, 2) NOT NULL DEFAULT 0,
    resources_text TEXT NOT NULL DEFAULT '',
    calc_status TEXT NOT NULL DEFAULT '',
    calc_error TEXT NOT NULL DEFAULT '',
    calculated_at TIMESTAMPTZ,
    PRIMARY KEY (estimate_id, generation, line_id)
);
CREATE INDEX IF NOT EXISTS idx_estimate_calc_lines_estimate_gen
    ON estimate_calc_lines(estimate_id, generation);
CREATE INDEX IF NOT EXISTS idx_estimate_calc_lines_line
    ON estimate_calc_lines(estimate_id, line_id);

CREATE TABLE IF NOT EXISTS estimate_calc_line_resources (
    estimate_id TEXT NOT NULL,
    generation BIGINT NOT NULL,
    line_id TEXT NOT NULL,
    resource_code TEXT NOT NULL,
    determinant TEXT NOT NULL DEFAULT '',
    consumption NUMERIC(18, 6) NOT NULL DEFAULT 0,
    name TEXT NOT NULL DEFAULT '',
    unit TEXT NOT NULL DEFAULT '',
    estimate_price NUMERIC(18, 4),
    selling_price NUMERIC(18, 4),
    transport_cost NUMERIC(18, 4),
    mass TEXT NOT NULL DEFAULT '',
    cargo_class TEXT NOT NULL DEFAULT '',
    corrections TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (estimate_id, generation, line_id, resource_code, determinant),
    FOREIGN KEY (estimate_id, generation, line_id)
        REFERENCES estimate_calc_lines(estimate_id, generation, line_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS estimate_calc_resources (
    estimate_id TEXT NOT NULL REFERENCES app_estimates(id) ON DELETE CASCADE,
    generation BIGINT NOT NULL,
    resource_code TEXT NOT NULL,
    determinant TEXT NOT NULL DEFAULT '',
    total_consumption NUMERIC(18, 6) NOT NULL DEFAULT 0,
    estimate_price NUMERIC(18, 4),
    selling_price NUMERIC(18, 4),
    transport_cost NUMERIC(18, 4),
    name TEXT NOT NULL DEFAULT '',
    unit TEXT NOT NULL DEFAULT '',
    mass TEXT NOT NULL DEFAULT '',
    cargo_class TEXT NOT NULL DEFAULT '',
    corrections TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (estimate_id, generation, resource_code, determinant)
);
CREATE INDEX IF NOT EXISTS idx_estimate_calc_resources_estimate_gen
    ON estimate_calc_resources(estimate_id, generation);
`

type lineCalcPersistInput struct {
	EstimateID   string
	Generation   int64
	LineID       string
	LineRevision int64
	PositionNo   int
	Snapshot     estimatecalc.LineCalcSnapshot
}

func lockEstimateCalcState(ctx context.Context, tx pgx.Tx, estimateID string) error {
	_, err := tx.Exec(ctx, `
INSERT INTO estimate_calc_state (estimate_id)
VALUES ($1)
ON CONFLICT (estimate_id) DO NOTHING
`, estimateID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
SELECT estimate_id FROM estimate_calc_state WHERE estimate_id = $1 FOR UPDATE
`, estimateID)
	return err
}

func subtractLineCalcContributionsTx(ctx context.Context, tx pgx.Tx, estimateID string, generation int64, lineID string) (oldTotal float64, hadRow bool, err error) {
	err = tx.QueryRow(ctx, `
SELECT total
FROM estimate_calc_lines
WHERE estimate_id = $1 AND generation = $2 AND line_id = $3 AND calc_status = 'done'
`, estimateID, generation, lineID).Scan(&oldTotal)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, false, nil
		}
		return 0, false, err
	}

	rows, err := tx.Query(ctx, `
SELECT resource_code, determinant, consumption
FROM estimate_calc_line_resources
WHERE estimate_id = $1 AND generation = $2 AND line_id = $3
`, estimateID, generation, lineID)
	if err != nil {
		return 0, false, err
	}
	defer rows.Close()

	type delta struct {
		code, determinant string
		consumption       float64
	}
	deltas := make([]delta, 0)
	for rows.Next() {
		var d delta
		if err := rows.Scan(&d.code, &d.determinant, &d.consumption); err != nil {
			return 0, false, err
		}
		deltas = append(deltas, d)
	}
	if err := rows.Err(); err != nil {
		return 0, false, err
	}

	for _, d := range deltas {
		tag, err := tx.Exec(ctx, `
UPDATE estimate_calc_resources
SET total_consumption = total_consumption - $5,
    updated_at = now()
WHERE estimate_id = $1 AND generation = $2 AND resource_code = $3 AND determinant = $4
`, estimateID, generation, d.code, d.determinant, d.consumption)
		if err != nil {
			return 0, false, err
		}
		if tag.RowsAffected() > 0 {
			if _, err := tx.Exec(ctx, `
DELETE FROM estimate_calc_resources
WHERE estimate_id = $1 AND generation = $2 AND resource_code = $3 AND determinant = $4
  AND total_consumption <= 0
`, estimateID, generation, d.code, d.determinant); err != nil {
				return 0, false, err
			}
		}
	}

	if _, err := tx.Exec(ctx, `
DELETE FROM estimate_calc_line_resources
WHERE estimate_id = $1 AND generation = $2 AND line_id = $3
`, estimateID, generation, lineID); err != nil {
		return 0, false, err
	}
	if _, err := tx.Exec(ctx, `
DELETE FROM estimate_calc_lines
WHERE estimate_id = $1 AND generation = $2 AND line_id = $3
`, estimateID, generation, lineID); err != nil {
		return 0, false, err
	}
	return oldTotal, true, nil
}

func addLineCalcContributionsTx(ctx context.Context, tx pgx.Tx, in lineCalcPersistInput) error {
	_, err := tx.Exec(ctx, `
INSERT INTO estimate_calc_lines (
    estimate_id, generation, line_id, line_revision, position_no,
    code, original_code, name, unit, quantity, unit_price, total,
    resources_text, calc_status, calc_error, calculated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10, $11, $12,
    $13, 'done', '', now()
)
ON CONFLICT (estimate_id, generation, line_id) DO UPDATE SET
    line_revision = EXCLUDED.line_revision,
    position_no = EXCLUDED.position_no,
    code = EXCLUDED.code,
    original_code = EXCLUDED.original_code,
    name = EXCLUDED.name,
    unit = EXCLUDED.unit,
    quantity = EXCLUDED.quantity,
    unit_price = EXCLUDED.unit_price,
    total = EXCLUDED.total,
    resources_text = EXCLUDED.resources_text,
    calc_status = 'done',
    calc_error = '',
    calculated_at = now()
`, in.EstimateID, in.Generation, in.LineID, in.LineRevision, in.PositionNo,
		in.Snapshot.Code, in.Snapshot.OriginalCode, in.Snapshot.Name, in.Snapshot.Unit,
		in.Snapshot.Quantity, in.Snapshot.UnitPrice, in.Snapshot.Total, in.Snapshot.ResourcesText)
	if err != nil {
		return err
	}

	for _, resource := range in.Snapshot.Resources {
		code := strings.TrimSpace(resource.Code)
		if code == "" {
			continue
		}
		determinant := strings.TrimSpace(resource.Determinant)
		if _, err := tx.Exec(ctx, `
INSERT INTO estimate_calc_line_resources (
    estimate_id, generation, line_id, resource_code, determinant,
    consumption, name, unit, estimate_price, selling_price, transport_cost,
    mass, cargo_class, corrections
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
`, in.EstimateID, in.Generation, in.LineID, code, determinant,
			resource.Consumption, resource.Name, resource.Unit,
			nullableFloat(resource.EstimatePrice), nullableFloatPtr(resource.SellingPrice), nullableFloatPtr(resource.TransportCost),
			resource.Mass, resource.CargoClass, resource.Corrections); err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `
INSERT INTO estimate_calc_resources (
    estimate_id, generation, resource_code, determinant,
    total_consumption, estimate_price, selling_price, transport_cost,
    name, unit, mass, cargo_class, corrections, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13, now())
ON CONFLICT (estimate_id, generation, resource_code, determinant) DO UPDATE SET
    total_consumption = estimate_calc_resources.total_consumption + EXCLUDED.total_consumption,
    estimate_price = COALESCE(EXCLUDED.estimate_price, estimate_calc_resources.estimate_price),
    selling_price = COALESCE(EXCLUDED.selling_price, estimate_calc_resources.selling_price),
    transport_cost = COALESCE(EXCLUDED.transport_cost, estimate_calc_resources.transport_cost),
    name = CASE WHEN EXCLUDED.name <> '' THEN EXCLUDED.name ELSE estimate_calc_resources.name END,
    unit = CASE WHEN EXCLUDED.unit <> '' THEN EXCLUDED.unit ELSE estimate_calc_resources.unit END,
    mass = CASE WHEN EXCLUDED.mass <> '' THEN EXCLUDED.mass ELSE estimate_calc_resources.mass END,
    cargo_class = CASE WHEN EXCLUDED.cargo_class <> '' THEN EXCLUDED.cargo_class ELSE estimate_calc_resources.cargo_class END,
    corrections = CASE WHEN EXCLUDED.corrections <> '' THEN EXCLUDED.corrections ELSE estimate_calc_resources.corrections END,
    updated_at = now()
`, in.EstimateID, in.Generation, code, determinant,
			resource.Consumption, nullableFloat(resource.EstimatePrice), nullableFloatPtr(resource.SellingPrice), nullableFloatPtr(resource.TransportCost),
			resource.Name, resource.Unit, resource.Mass, resource.CargoClass, resource.Corrections); err != nil {
			return err
		}
	}
	return nil
}

func applyDoneLineCalcTx(ctx context.Context, tx pgx.Tx, in lineCalcPersistInput) error {
	if err := lockEstimateCalcState(ctx, tx, in.EstimateID); err != nil {
		return fmt.Errorf("lock calc state: %w", err)
	}

	oldTotal, hadOld, err := subtractLineCalcContributionsTx(ctx, tx, in.EstimateID, in.Generation, in.LineID)
	if err != nil {
		return fmt.Errorf("subtract old line calc: %w", err)
	}
	if err := addLineCalcContributionsTx(ctx, tx, in); err != nil {
		return fmt.Errorf("add line calc: %w", err)
	}

	deltaDone := 0
	if !hadOld {
		deltaDone = 1
	}
	_, err = tx.Exec(ctx, `
UPDATE estimate_calc_state
SET generation = $2,
    grand_total = grand_total - $3 + $4,
    lines_done = GREATEST(0, lines_done + $5),
    status = CASE
        WHEN lines_total > 0 AND GREATEST(0, lines_done + $5) + lines_errors >= lines_total THEN 'done'
        ELSE 'running'
    END,
    updated_at = now()
WHERE estimate_id = $1
`, in.EstimateID, in.Generation, oldTotal, in.Snapshot.Total, deltaDone)
	return err
}

func (s *FileStore) reconcileEstimateCalcStateIfIdle(ctx context.Context, estimateID string) error {
	if s.treeDB == nil {
		return nil
	}
	tx, err := s.treeDB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `
SELECT COALESCE(status, '')
FROM estimate_calc_state
WHERE estimate_id = $1
FOR UPDATE
`, estimateID).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	status = strings.TrimSpace(status)
	if status != "running" && status != "starting" {
		return nil
	}
	if err := reconcileEstimateCalcStateTx(ctx, tx, estimateID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func reconcileEstimateCalcStateTx(ctx context.Context, tx pgx.Tx, estimateID string) error {
	var pendingLines, pendingJobs, pendingOutbox int
	err := tx.QueryRow(ctx, `
SELECT
    (SELECT COUNT(*)::int
     FROM app_estimate_lines
     WHERE estimate_id = $1
       AND line_type = $2
       AND source = 'gsn'
       AND calc_status IN ('queued', 'leased')
       AND COALESCE(NULLIF(TRIM(code), ''), NULLIF(TRIM(original_code), '')) IS NOT NULL),
    (SELECT COUNT(*)::int
     FROM estimate_calc_jobs
     WHERE estimate_id = $1
       AND status IN ('queued', 'leased')),
    (SELECT COUNT(*)::int
     FROM outbox_events
     WHERE published_at IS NULL
       AND payload->>'estimateId' = $1)
`, estimateID, string(domain.EstimateLinePosition)).Scan(&pendingLines, &pendingJobs, &pendingOutbox)
	if err != nil {
		return err
	}
	if pendingLines > 0 || pendingJobs > 0 || pendingOutbox > 0 {
		return nil
	}

	var total, done, failed int
	var grandTotal float64
	err = tx.QueryRow(ctx, `
SELECT
    COUNT(*)::int,
    COUNT(*) FILTER (WHERE calc_status = 'done')::int,
    COUNT(*) FILTER (WHERE calc_status IN ('failed', 'dead'))::int,
    COALESCE(SUM(total) FILTER (WHERE calc_status = 'done'), 0)
FROM app_estimate_lines
WHERE estimate_id = $1
    AND line_type = $2
    AND source = 'gsn'
    AND COALESCE(NULLIF(TRIM(code), ''), NULLIF(TRIM(original_code), '')) IS NOT NULL
`, estimateID, string(domain.EstimateLinePosition)).Scan(&total, &done, &failed, &grandTotal)
	if err != nil {
		return err
	}

	newStatus := "running"
	if total == 0 || done+failed >= total {
		newStatus = "done"
	}

	_, err = tx.Exec(ctx, `
UPDATE estimate_calc_state
SET lines_total = $2,
    lines_done = $3,
    lines_errors = $4,
    grand_total = $5,
    status = $6,
    updated_at = now()
WHERE estimate_id = $1
  AND status IN ('running', 'starting', 'done')
`, estimateID, total, done, failed, grandTotal, newStatus)
	return err
}

func removeLineCalcFromGenerationTx(ctx context.Context, tx pgx.Tx, estimateID string, generation int64, lineID string) error {
	if err := lockEstimateCalcState(ctx, tx, estimateID); err != nil {
		return err
	}
	oldTotal, hadOld, err := subtractLineCalcContributionsTx(ctx, tx, estimateID, generation, lineID)
	if err != nil {
		return err
	}
	if !hadOld {
		return nil
	}
	_, err = tx.Exec(ctx, `
UPDATE estimate_calc_state
SET grand_total = GREATEST(0, grand_total - $2),
    lines_done = GREATEST(0, lines_done - 1),
    lines_total = GREATEST(0, lines_total - 1),
    updated_at = now()
WHERE estimate_id = $1
`, estimateID, oldTotal)
	return err
}

func clearEstimateCalcGenerationTx(ctx context.Context, tx pgx.Tx, estimateID string, generation int64, district, fgisSetID string, linesTotal int) error {
	if _, err := tx.Exec(ctx, `DELETE FROM estimate_calc_line_resources WHERE estimate_id = $1`, estimateID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM estimate_calc_lines WHERE estimate_id = $1`, estimateID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM estimate_calc_resources WHERE estimate_id = $1`, estimateID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
INSERT INTO estimate_calc_state (
    estimate_id, generation, district, fgis_set_id, status,
    grand_total, lines_total, lines_done, lines_errors, updated_at
) VALUES ($1, $2, $3, $4, 'running', 0, $5, 0, 0, now())
ON CONFLICT (estimate_id) DO UPDATE SET
    generation = EXCLUDED.generation,
    district = EXCLUDED.district,
    fgis_set_id = EXCLUDED.fgis_set_id,
    status = 'running',
    grand_total = 0,
    lines_total = EXCLUDED.lines_total,
    lines_done = 0,
    lines_errors = 0,
    updated_at = now()
`, estimateID, generation, district, fgisSetID, linesTotal)
	return err
}

func nullableFloat(value float64) any {
	return value
}

func nullableFloatPtr(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func loadLineSortOrder(ctx context.Context, tx pgx.Tx, estimateID, lineID string) (int, error) {
	var sortOrder int
	err := tx.QueryRow(ctx, `
SELECT sort_order FROM app_estimate_lines WHERE estimate_id = $1 AND id = $2
`, estimateID, lineID).Scan(&sortOrder)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return sortOrder, err
}

func loadEstimateGeneration(ctx context.Context, tx pgx.Tx, estimateID string) (int64, error) {
	var generation int64
	err := tx.QueryRow(ctx, `SELECT calc_generation FROM app_estimates WHERE id = $1`, estimateID).Scan(&generation)
	return generation, err
}

// HasEstimateCalcLines reports whether any calc line rows exist for the estimate.
func (s *FileStore) HasEstimateCalcLines(ctx context.Context, estimateID string) (bool, error) {
	if s.treeDB == nil {
		return false, nil
	}
	var exists bool
	err := s.treeDB.QueryRow(ctx, `
SELECT EXISTS (SELECT 1 FROM estimate_calc_lines WHERE estimate_id = $1 LIMIT 1)
`, estimateID).Scan(&exists)
	return exists, err
}

