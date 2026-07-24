-- Assign determinant 12 to all GSN positions under subsection
-- "Логистические процессы" (hierarchy 04-00-00-00-00-000-00, доп.18).
-- Safe to re-run: skips rows already correct.

WITH RECURSIVE logistics AS (
    SELECT supplement_code, code
    FROM gsn.hierarchy
    WHERE supplement_code = 'доп.18'
      AND code = '04-00-00-00-00-000-00'
    UNION ALL
    SELECT h.supplement_code, h.code
    FROM gsn.hierarchy h
    JOIN logistics t
      ON h.supplement_code = t.supplement_code
     AND h.parent_code = t.code
),
log_recs AS (
    SELECT DISTINCT refs.record_code
    FROM logistics
    JOIN gsn.hierarchy_record_refs refs
      ON refs.supplement_code = logistics.supplement_code
     AND refs.hierarchy_code = logistics.code
)
UPDATE gsn.records r
SET
    determinant = '12',
    -- Field layout: code'determinant'cost'name...
    -- Fixes both original empty form (CODE'''name) and prior bad rewrite (CODE'12'''name).
    raw_line = CASE
        WHEN r.raw_line ~ $re$^[^']+'12'''$re$
            THEN regexp_replace(r.raw_line, $re$^([^']+)'12'''$re$, $re$\1'12''$re$)
        WHEN r.raw_line ~ $re$^[^']+'''$re$
            THEN regexp_replace(r.raw_line, $re$^([^']+)'''$re$, $re$\1'12''$re$)
        ELSE r.raw_line
    END
FROM log_recs
WHERE r.code = log_recs.record_code
  AND (
      r.determinant IS DISTINCT FROM '12'
      OR r.raw_line ~ $re$^[^']+'12'''$re$
      OR (
          r.raw_line ~ $re$^[^']+'''$re$
          AND r.raw_line !~ $re$^[^']+'12''$re$
      )
  );
