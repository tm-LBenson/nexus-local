ALTER TABLE data_source_scan_entries
    ADD COLUMN IF NOT EXISTS content_hash text NOT NULL DEFAULT '';
