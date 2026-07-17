-- Allow transient 'starting' status while calc batches are being enqueued.
ALTER TABLE estimate_calc_state DROP CONSTRAINT IF EXISTS estimate_calc_state_status_check;
ALTER TABLE estimate_calc_state ADD CONSTRAINT estimate_calc_state_status_check
    CHECK (status IN ('', 'starting', 'running', 'done', 'failed'));
