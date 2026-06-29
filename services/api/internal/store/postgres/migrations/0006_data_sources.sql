CREATE TABLE IF NOT EXISTS data_sources (
    tenant_id text NOT NULL,
    id text NOT NULL,
    owner_id text NOT NULL,
    type text NOT NULL,
    name text NOT NULL,
    root_path text NOT NULL,
    status text NOT NULL,
    last_scan_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS data_sources_tenant_updated_idx ON data_sources (tenant_id, updated_at DESC, id);
CREATE INDEX IF NOT EXISTS data_sources_tenant_status_idx ON data_sources (tenant_id, status, updated_at DESC);
