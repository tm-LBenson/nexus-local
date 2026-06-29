CREATE TABLE IF NOT EXISTS data_source_scan_entries (
    tenant_id text NOT NULL,
    job_id text NOT NULL,
    source_id text NOT NULL,
    path text NOT NULL,
    outcome text NOT NULL,
    reason text NOT NULL DEFAULT '',
    message text NOT NULL DEFAULT '',
    document_id text NOT NULL DEFAULT '',
    size_bytes bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, job_id, path)
);

CREATE INDEX IF NOT EXISTS data_source_scan_entries_source_idx
    ON data_source_scan_entries (tenant_id, source_id, created_at DESC, path);
