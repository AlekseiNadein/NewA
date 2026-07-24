-- Include estimate_price in estimate_calc_resources primary key (ADR-004).
-- Rebuilds aggregates from line-level rows so merged price-list rows split correctly.
-- Runtime also applies this via ensureTreeSchema on server start.

ALTER TABLE estimate_calc_resources DROP CONSTRAINT IF EXISTS estimate_calc_resources_pkey;
ALTER TABLE estimate_calc_resources ALTER COLUMN estimate_price SET DEFAULT 0;
UPDATE estimate_calc_resources SET estimate_price = COALESCE(estimate_price, 0);
ALTER TABLE estimate_calc_resources ALTER COLUMN estimate_price SET NOT NULL;
DELETE FROM estimate_calc_resources;
INSERT INTO estimate_calc_resources (
    estimate_id, generation, resource_code, determinant,
    total_consumption, estimate_price, selling_price, transport_cost,
    name, unit, mass, cargo_class, corrections, updated_at
)
SELECT
    lr.estimate_id,
    lr.generation,
    lr.resource_code,
    lr.determinant,
    SUM(lr.consumption),
    COALESCE(lr.estimate_price, 0),
    MAX(lr.selling_price),
    MAX(lr.transport_cost),
    COALESCE(MAX(lr.name) FILTER (WHERE lr.name <> ''), ''),
    COALESCE(MAX(lr.unit) FILTER (WHERE lr.unit <> ''), ''),
    COALESCE(MAX(lr.mass) FILTER (WHERE lr.mass <> ''), ''),
    COALESCE(MAX(lr.cargo_class) FILTER (WHERE lr.cargo_class <> ''), ''),
    COALESCE(MAX(lr.corrections) FILTER (WHERE lr.corrections <> ''), ''),
    now()
FROM estimate_calc_line_resources lr
GROUP BY lr.estimate_id, lr.generation, lr.resource_code, lr.determinant, COALESCE(lr.estimate_price, 0);
ALTER TABLE estimate_calc_resources
    ADD PRIMARY KEY (estimate_id, generation, resource_code, determinant, estimate_price);
