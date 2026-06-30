CREATE TABLE IF NOT EXISTS source_policy_profiles (
    tenant_id text NOT NULL,
    id text NOT NULL,
    owner_id text NOT NULL,
    name text NOT NULL,
    detail text NOT NULL DEFAULT '',
    include_patterns text[] NOT NULL DEFAULT '{}',
    exclude_patterns text[] NOT NULL DEFAULT '{}',
    scan_interval_minutes integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS source_policy_profiles_tenant_name_idx
    ON source_policy_profiles (tenant_id, lower(name), id);
