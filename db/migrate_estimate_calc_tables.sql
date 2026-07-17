-- Estimate calculation state and aggregates.
-- Safe to run once; also applied via ensureTreeSchema.

CREATE TABLE IF NOT EXISTS estimate_calc_state (
    estimate_id TEXT PRIMARY KEY REFERENCES app_estimates(id) ON DELETE CASCADE,
    generation BIGINT NOT NULL DEFAULT 0,
    district TEXT NOT NULL DEFAULT '',
    fgis_set_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '' CHECK (status IN ('', 'running', 'done', 'failed')),
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

