ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS resource_type text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS resource_id text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS jobs_resource_idx ON jobs (tenant_id, resource_type, resource_id);

