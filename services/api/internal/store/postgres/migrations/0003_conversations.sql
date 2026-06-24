CREATE TABLE IF NOT EXISTS conversations (
    tenant_id text NOT NULL,
    id text NOT NULL,
    owner_id text NOT NULL,
    title text NOT NULL DEFAULT '',
    model_target text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS conversations_tenant_updated_idx ON conversations (tenant_id, updated_at DESC, id);

CREATE TABLE IF NOT EXISTS messages (
    tenant_id text NOT NULL,
    id text NOT NULL,
    conversation_id text NOT NULL,
    role text NOT NULL,
    content text NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, conversation_id) REFERENCES conversations (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS messages_conversation_created_idx ON messages (tenant_id, conversation_id, created_at, id);
