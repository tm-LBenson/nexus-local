CREATE TABLE IF NOT EXISTS tenants (
    id text PRIMARY KEY,
    name text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
    id text PRIMARY KEY,
    email text NOT NULL UNIQUE,
    name text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS memberships (
    tenant_id text NOT NULL,
    user_id text NOT NULL,
    role text NOT NULL,
    PRIMARY KEY (tenant_id, user_id)
);

CREATE INDEX IF NOT EXISTS memberships_user_id_idx ON memberships (user_id);

CREATE TABLE IF NOT EXISTS documents (
    tenant_id text NOT NULL,
    id text NOT NULL,
    owner_id text NOT NULL,
    name text NOT NULL,
    storage_key text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    status text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS documents_tenant_created_idx ON documents (tenant_id, created_at, id);

CREATE TABLE IF NOT EXISTS jobs (
    tenant_id text NOT NULL,
    id text NOT NULL,
    type text NOT NULL,
    state text NOT NULL,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS jobs_queue_idx ON jobs (state, created_at, id);

