ALTER TABLE data_sources
    ADD COLUMN IF NOT EXISTS scan_interval_minutes integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS next_scan_at timestamptz;

CREATE INDEX IF NOT EXISTS data_sources_due_scan_idx
    ON data_sources (next_scan_at, tenant_id, id)
    WHERE scan_interval_minutes > 0 AND next_scan_at IS NOT NULL AND status IN ('active', 'failed');
