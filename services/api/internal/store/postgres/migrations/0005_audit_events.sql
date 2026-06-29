CREATE TABLE IF NOT EXISTS audit_events (
    tenant_id text NOT NULL,
    id text NOT NULL,
    actor_user_id text NOT NULL,
    action text NOT NULL,
    resource_type text NOT NULL DEFAULT '',
    resource_id text NOT NULL DEFAULT '',
    outcome text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS audit_events_tenant_created_idx ON audit_events (tenant_id, created_at DESC, id);
