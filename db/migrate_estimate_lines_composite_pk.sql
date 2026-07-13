-- Migrate app_estimate_lines primary key from global id to (estimate_id, id).
-- Safe to run once; runtime also applies this via ensureTreeSchema on server start.

ALTER TABLE app_estimate_lines DROP CONSTRAINT IF EXISTS app_estimate_lines_pkey;
ALTER TABLE app_estimate_lines ADD PRIMARY KEY (estimate_id, id);
