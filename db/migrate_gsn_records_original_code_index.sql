-- Speed up lookupRecordDetail fallback: WHERE original_code = $1
-- Safe to run multiple times (IF NOT EXISTS).

CREATE INDEX IF NOT EXISTS idx_gsn_records_original_code
    ON gsn.records(original_code) WHERE original_code <> '';
