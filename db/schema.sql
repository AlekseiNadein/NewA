-- App contour: constructions, estimates, calc queue, settings.
-- Auth data lives in auth.* (see db/auth_schema.sql).

CREATE TABLE IF NOT EXISTS app_constructions (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_constructions_company_id ON app_constructions(company_id);

CREATE TABLE IF NOT EXISTS app_construction_objects (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL,
    construction_id TEXT NOT NULL REFERENCES app_constructions(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_construction_objects_company_id ON app_construction_objects(company_id);
CREATE INDEX IF NOT EXISTS idx_app_construction_objects_construction_id ON app_construction_objects(construction_id);

CREATE TABLE IF NOT EXISTS app_estimates (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL,
    object_id TEXT NOT NULL REFERENCES app_construction_objects(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    district TEXT NOT NULL DEFAULT '',
    fgis_set_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('draft', 'approved', 'archived')),
    total NUMERIC(14, 2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_estimates_company_id ON app_estimates(company_id);
CREATE INDEX IF NOT EXISTS idx_app_estimates_object_id ON app_estimates(object_id);

CREATE TABLE IF NOT EXISTS app_estimate_lines (
    id TEXT NOT NULL,
    estimate_id TEXT NOT NULL REFERENCES app_estimates(id) ON DELETE CASCADE,
    line_type TEXT NOT NULL CHECK (line_type IN ('section', 'subsection', 'position')),
    source TEXT NOT NULL DEFAULT '',
    code TEXT NOT NULL DEFAULT '',
    original_code TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    quantity NUMERIC(14, 3) NOT NULL DEFAULT 0,
    unit TEXT NOT NULL DEFAULT '',
    unit_price NUMERIC(14, 2) NOT NULL DEFAULT 0,
    total NUMERIC(14, 2) NOT NULL DEFAULT 0,
    raw_text TEXT NOT NULL DEFAULT '',
    parsed_json JSONB,
    calc_json JSONB,
    calc_status TEXT NOT NULL DEFAULT '',
    calc_error TEXT NOT NULL DEFAULT '',
    revision BIGINT NOT NULL DEFAULT 0,
    calculated_at TIMESTAMPTZ,
    sort_order INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (estimate_id, id)
);

CREATE INDEX IF NOT EXISTS idx_app_estimate_lines_estimate_id ON app_estimate_lines(estimate_id, sort_order, id);

CREATE TABLE IF NOT EXISTS app_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS estimate_calc_jobs (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL,
    estimate_id TEXT NOT NULL REFERENCES app_estimates(id) ON DELETE CASCADE,
    line_id TEXT NOT NULL,
    revision BIGINT NOT NULL,
    job_type TEXT NOT NULL DEFAULT 'estimate_line',
    status TEXT NOT NULL CHECK (status IN ('queued', 'leased', 'done', 'failed', 'dead')),
    priority INTEGER NOT NULL DEFAULT 0,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    run_after TIMESTAMPTZ NOT NULL DEFAULT now(),
    leased_until TIMESTAMPTZ,
    locked_by TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (estimate_id, line_id, revision)
);

CREATE INDEX IF NOT EXISTS idx_estimate_calc_jobs_ready
    ON estimate_calc_jobs(priority DESC, run_after, created_at)
    WHERE status = 'queued';

CREATE INDEX IF NOT EXISTS idx_estimate_calc_jobs_estimate
    ON estimate_calc_jobs(estimate_id, line_id, revision);

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
