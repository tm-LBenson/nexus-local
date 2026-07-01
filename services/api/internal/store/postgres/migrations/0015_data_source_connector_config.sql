ALTER TABLE data_sources
    ADD COLUMN IF NOT EXISTS connector_config jsonb NOT NULL DEFAULT '{}'::jsonb;
