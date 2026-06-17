-- Миграция: уровень дополнений (доп. N) над иерархией ГСН.
-- Применять к существующей БД с импортом доп. 18.

BEGIN;

CREATE TABLE IF NOT EXISTS gsn.supplements (
    code TEXT PRIMARY KEY,
    label TEXT NOT NULL,
    edition TEXT NOT NULL DEFAULT '',
    version_date TEXT NOT NULL DEFAULT '',
    ordinal INTEGER NOT NULL DEFAULT 0,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO gsn.supplements (code, label, edition, version_date, ordinal)
SELECT
    'доп.18',
    'доп. 18',
    COALESCE((SELECT param_value FROM gsn.base_info_params WHERE param_key = 'Редакция СНБ' LIMIT 1), 'ГЭСН-2022 доп.18'),
    COALESCE((SELECT param_value FROM gsn.base_info_params WHERE param_key = 'Версия' LIMIT 1), ''),
    18
ON CONFLICT (code) DO NOTHING;

DROP VIEW IF EXISTS gsn.base_info_json;

ALTER TABLE gsn.base_info_params DROP CONSTRAINT IF EXISTS base_info_params_base_code_fkey;
ALTER TABLE gsn.base_info DROP CONSTRAINT IF EXISTS base_info_pkey;

ALTER TABLE gsn.base_info ADD COLUMN IF NOT EXISTS supplement_code TEXT;
UPDATE gsn.base_info SET supplement_code = 'доп.18' WHERE supplement_code IS NULL;
ALTER TABLE gsn.base_info ALTER COLUMN supplement_code SET NOT NULL;

ALTER TABLE gsn.base_info DROP CONSTRAINT IF EXISTS base_info_supplement_code_fkey;
ALTER TABLE gsn.base_info DROP CONSTRAINT IF EXISTS base_info_pkey;
ALTER TABLE gsn.base_info
    ADD CONSTRAINT base_info_pkey PRIMARY KEY (supplement_code, code);
ALTER TABLE gsn.base_info
    ADD CONSTRAINT base_info_supplement_code_fkey
    FOREIGN KEY (supplement_code) REFERENCES gsn.supplements(code) ON DELETE CASCADE;

ALTER TABLE gsn.base_info_params ADD COLUMN IF NOT EXISTS supplement_code TEXT;
UPDATE gsn.base_info_params bip
SET supplement_code = bi.supplement_code
FROM gsn.base_info bi
WHERE bi.code = bip.base_code AND bip.supplement_code IS NULL;
ALTER TABLE gsn.base_info_params ALTER COLUMN supplement_code SET NOT NULL;

ALTER TABLE gsn.base_info_params DROP CONSTRAINT IF EXISTS base_info_params_pkey;
ALTER TABLE gsn.base_info_params
    ADD CONSTRAINT base_info_params_pkey PRIMARY KEY (supplement_code, base_code, ordinal);
ALTER TABLE gsn.base_info_params
    ADD CONSTRAINT base_info_params_base_fkey
    FOREIGN KEY (supplement_code, base_code)
    REFERENCES gsn.base_info(supplement_code, code) ON DELETE CASCADE;

ALTER TABLE gsn.hierarchy ADD COLUMN IF NOT EXISTS supplement_code TEXT;
UPDATE gsn.hierarchy SET supplement_code = 'доп.18' WHERE supplement_code IS NULL;
ALTER TABLE gsn.hierarchy ALTER COLUMN supplement_code SET NOT NULL;

ALTER TABLE gsn.hierarchy_record_refs DROP CONSTRAINT IF EXISTS hierarchy_record_refs_hierarchy_code_fkey;
ALTER TABLE gsn.hierarchy_record_refs DROP CONSTRAINT IF EXISTS hierarchy_record_refs_hierarchy_fkey;
ALTER TABLE gsn.hierarchy DROP CONSTRAINT IF EXISTS hierarchy_parent_code_fkey;
ALTER TABLE gsn.hierarchy DROP CONSTRAINT IF EXISTS hierarchy_parent_fkey;
ALTER TABLE gsn.hierarchy DROP CONSTRAINT IF EXISTS hierarchy_pkey;

ALTER TABLE gsn.hierarchy
    ADD CONSTRAINT hierarchy_pkey PRIMARY KEY (supplement_code, code);

ALTER TABLE gsn.hierarchy
    ADD CONSTRAINT hierarchy_parent_fkey
    FOREIGN KEY (supplement_code, parent_code)
    REFERENCES gsn.hierarchy(supplement_code, code) ON DELETE CASCADE;

ALTER TABLE gsn.hierarchy_record_refs ADD COLUMN IF NOT EXISTS supplement_code TEXT;
UPDATE gsn.hierarchy_record_refs refs
SET supplement_code = h.supplement_code
FROM gsn.hierarchy h
WHERE h.code = refs.hierarchy_code AND refs.supplement_code IS NULL;
ALTER TABLE gsn.hierarchy_record_refs ALTER COLUMN supplement_code SET NOT NULL;

ALTER TABLE gsn.hierarchy_record_refs DROP CONSTRAINT IF EXISTS hierarchy_record_refs_pkey;
ALTER TABLE gsn.hierarchy_record_refs
    ADD CONSTRAINT hierarchy_record_refs_pkey
    PRIMARY KEY (supplement_code, hierarchy_code, record_code, ordinal);

ALTER TABLE gsn.hierarchy_record_refs
    ADD CONSTRAINT hierarchy_record_refs_hierarchy_fkey
    FOREIGN KEY (supplement_code, hierarchy_code)
    REFERENCES gsn.hierarchy(supplement_code, code) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_gsn_hierarchy_supplement_parent
    ON gsn.hierarchy(supplement_code, parent_code);

CREATE INDEX IF NOT EXISTS idx_gsn_base_info_supplement
    ON gsn.base_info(supplement_code);

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

COMMIT;
