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
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_constructions_company_id ON constructions(company_id);

CREATE TABLE construction_objects (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    construction_id TEXT NOT NULL REFERENCES constructions(id) ON DELETE CASCADE,
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
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
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
