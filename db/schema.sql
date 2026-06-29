CREATE TABLE companies (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('super_admin', 'company_admin', 'user')),
    password_hash TEXT NOT NULL,
    password_salt TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_company_id ON users(company_id);

CREATE TABLE constructions (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_constructions_company_id ON constructions(company_id);

CREATE TABLE construction_objects (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    construction_id TEXT NOT NULL REFERENCES constructions(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_construction_objects_company_id ON construction_objects(company_id);
CREATE INDEX idx_construction_objects_construction_id ON construction_objects(construction_id);

CREATE TABLE estimates (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    object_id TEXT NOT NULL REFERENCES construction_objects(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    district TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('draft', 'approved', 'archived')),
    total NUMERIC(14, 2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_estimates_company_id ON estimates(company_id);
CREATE INDEX idx_estimates_object_id ON estimates(object_id);

CREATE TABLE estimate_items (
    id TEXT PRIMARY KEY,
    estimate_id TEXT NOT NULL REFERENCES estimates(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    quantity NUMERIC(14, 3) NOT NULL,
    unit TEXT NOT NULL,
    unit_price NUMERIC(14, 2) NOT NULL,
    total NUMERIC(14, 2) NOT NULL
);

CREATE INDEX idx_estimate_items_estimate_id ON estimate_items(estimate_id);

CREATE TABLE outbox_events (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_outbox_events_unprocessed ON outbox_events(created_at) WHERE processed_at IS NULL;

CREATE TABLE app_constructions (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_app_constructions_company_id ON app_constructions(company_id);

CREATE TABLE app_construction_objects (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL,
    construction_id TEXT NOT NULL REFERENCES app_constructions(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_app_construction_objects_company_id ON app_construction_objects(company_id);
CREATE INDEX idx_app_construction_objects_construction_id ON app_construction_objects(construction_id);

CREATE TABLE app_estimates (
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

CREATE INDEX idx_app_estimates_company_id ON app_estimates(company_id);
CREATE INDEX idx_app_estimates_object_id ON app_estimates(object_id);

CREATE TABLE app_estimate_lines (
    id TEXT PRIMARY KEY,
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
    sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_app_estimate_lines_estimate_id ON app_estimate_lines(estimate_id, sort_order, id);

CREATE TABLE app_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE estimate_calc_jobs (
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

CREATE INDEX idx_estimate_calc_jobs_ready
    ON estimate_calc_jobs(priority DESC, run_after, created_at)
    WHERE status = 'queued';

CREATE INDEX idx_estimate_calc_jobs_estimate
    ON estimate_calc_jobs(estimate_id, line_id, revision);
