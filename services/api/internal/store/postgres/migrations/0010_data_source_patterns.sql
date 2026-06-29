ALTER TABLE data_sources
    ADD COLUMN IF NOT EXISTS include_patterns text[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS exclude_patterns text[] NOT NULL DEFAULT '{}';
