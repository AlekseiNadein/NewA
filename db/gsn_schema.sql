CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE SCHEMA IF NOT EXISTS gsn;

CREATE TABLE IF NOT EXISTS gsn.supplements (
    code TEXT PRIMARY KEY,
    label TEXT NOT NULL,
    edition TEXT NOT NULL DEFAULT '',
    version_date TEXT NOT NULL DEFAULT '',
    ordinal INTEGER NOT NULL DEFAULT 0,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS gsn.import_batches (
    id TEXT PRIMARY KEY,
    source_path TEXT NOT NULL,
    prepared_path TEXT,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS gsn.base_info (
    supplement_code TEXT NOT NULL REFERENCES gsn.supplements(code) ON DELETE CASCADE,
    code TEXT NOT NULL,
    source_file TEXT NOT NULL,
    line_no INTEGER NOT NULL,
    raw_line TEXT NOT NULL,
    PRIMARY KEY (supplement_code, code)
);

CREATE TABLE IF NOT EXISTS gsn.base_info_params (
    supplement_code TEXT NOT NULL,
    base_code TEXT NOT NULL,
    param_key TEXT NOT NULL,
    param_value TEXT NOT NULL DEFAULT '',
    ordinal INTEGER NOT NULL,
    PRIMARY KEY (supplement_code, base_code, ordinal),
    FOREIGN KEY (supplement_code, base_code)
        REFERENCES gsn.base_info(supplement_code, code) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_gsn_base_info_params_key
    ON gsn.base_info_params(param_key);

CREATE OR REPLACE VIEW gsn.base_info_json AS
SELECT
    bi.supplement_code,
    bi.code,
    bi.source_file,
    bi.line_no,
    jsonb_object_agg(bip.param_key, bip.param_value ORDER BY bip.ordinal) AS params,
    bi.raw_line
FROM gsn.base_info bi
JOIN gsn.base_info_params bip
    ON bip.supplement_code = bi.supplement_code AND bip.base_code = bi.code
GROUP BY bi.supplement_code, bi.code, bi.source_file, bi.line_no, bi.raw_line;

CREATE TABLE IF NOT EXISTS gsn.hierarchy (
    supplement_code TEXT NOT NULL REFERENCES gsn.supplements(code) ON DELETE CASCADE,
    code TEXT NOT NULL,
    parent_code TEXT,
    line_no INTEGER NOT NULL,
    level INTEGER NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    unit TEXT NOT NULL DEFAULT '',
    raw_norm_list TEXT NOT NULL DEFAULT '',
    raw_line TEXT NOT NULL,
    PRIMARY KEY (supplement_code, code),
    FOREIGN KEY (supplement_code, parent_code)
        REFERENCES gsn.hierarchy(supplement_code, code) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_gsn_hierarchy_supplement_parent
    ON gsn.hierarchy(supplement_code, parent_code);
CREATE INDEX IF NOT EXISTS idx_gsn_hierarchy_level ON gsn.hierarchy(level);

CREATE TABLE IF NOT EXISTS gsn.hierarchy_record_refs (
    supplement_code TEXT NOT NULL,
    hierarchy_code TEXT NOT NULL,
    record_code TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    PRIMARY KEY (supplement_code, hierarchy_code, record_code, ordinal),
    FOREIGN KEY (supplement_code, hierarchy_code)
        REFERENCES gsn.hierarchy(supplement_code, code) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_gsn_hierarchy_record_refs_record_code
    ON gsn.hierarchy_record_refs(record_code);

CREATE TABLE IF NOT EXISTS gsn.records (
    code TEXT PRIMARY KEY,
    source_file TEXT NOT NULL,
    line_no INTEGER NOT NULL,
    original_code TEXT NOT NULL DEFAULT '',
    determinant TEXT NOT NULL DEFAULT '',
    cost_indicators TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL DEFAULT '',
    unit TEXT NOT NULL DEFAULT '',
    mass TEXT NOT NULL DEFAULT '',
    resource_list TEXT NOT NULL DEFAULT '',
    record_kind TEXT NOT NULL DEFAULT '',
    raw_line TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_gsn_records_source_file ON gsn.records(source_file);
CREATE INDEX IF NOT EXISTS idx_gsn_records_kind ON gsn.records(record_kind);
CREATE INDEX IF NOT EXISTS idx_gsn_records_name_trgm ON gsn.records USING gin (name gin_trgm_ops);

CREATE TABLE IF NOT EXISTS gsn.record_resources (
    record_code TEXT NOT NULL REFERENCES gsn.records(code) ON DELETE CASCADE,
    resource_code TEXT NOT NULL,
    quantity_text TEXT NOT NULL DEFAULT '',
    ordinal INTEGER NOT NULL,
    PRIMARY KEY (record_code, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_gsn_record_resources_resource_code
    ON gsn.record_resources(resource_code);

CREATE TABLE IF NOT EXISTS gsn.nsi (
    record_code TEXT NOT NULL,
    line_no INTEGER NOT NULL,
    original_code TEXT NOT NULL DEFAULT '',
    modifiers TEXT NOT NULL DEFAULT '',
    cost_indicators TEXT NOT NULL DEFAULT '',
    work_composition TEXT NOT NULL DEFAULT '',
    raw_line TEXT NOT NULL,
    PRIMARY KEY (record_code, line_no)
);

CREATE INDEX IF NOT EXISTS idx_gsn_nsi_record_code ON gsn.nsi(record_code);

CREATE TABLE IF NOT EXISTS gsn.amendments (
    code TEXT PRIMARY KEY,
    norm_code_addition TEXT NOT NULL DEFAULT '',
    name_addition TEXT NOT NULL DEFAULT '',
    interface_name TEXT NOT NULL DEFAULT '',
    raw_line TEXT NOT NULL,
    line_no INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS gsn.amendment_impacts (
    amendment_code TEXT NOT NULL REFERENCES gsn.amendments(code) ON DELETE CASCADE,
    impact_code TEXT NOT NULL,
    impact_value TEXT NOT NULL DEFAULT '',
    ordinal INTEGER NOT NULL,
    PRIMARY KEY (amendment_code, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_gsn_amendment_impacts_impact_code
    ON gsn.amendment_impacts(impact_code);

CREATE TABLE IF NOT EXISTS gsn.record_amendments (
    record_code TEXT NOT NULL,
    amendment_code TEXT NOT NULL,
    line_no INTEGER NOT NULL,
    ordinal INTEGER NOT NULL,
    PRIMARY KEY (record_code, amendment_code, ordinal)
);

ALTER TABLE gsn.record_amendments
    DROP CONSTRAINT IF EXISTS record_amendments_amendment_code_fkey;

CREATE INDEX IF NOT EXISTS idx_gsn_record_amendments_record_code
    ON gsn.record_amendments(record_code);

CREATE INDEX IF NOT EXISTS idx_gsn_record_amendments_amendment_code
    ON gsn.record_amendments(amendment_code);

