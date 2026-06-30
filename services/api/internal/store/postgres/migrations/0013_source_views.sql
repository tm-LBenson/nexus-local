CREATE TABLE IF NOT EXISTS source_views (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    id TEXT NOT NULL,
    owner_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    health_filter TEXT NOT NULL DEFAULT '',
    query_filter TEXT NOT NULL DEFAULT '',
    schedule_filter TEXT NOT NULL DEFAULT '',
    type_filter TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS source_views_tenant_name_idx
    ON source_views (tenant_id, lower(name), id);
