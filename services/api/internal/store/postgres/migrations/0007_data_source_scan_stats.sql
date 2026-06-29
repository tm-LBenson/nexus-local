ALTER TABLE data_sources
    ADD COLUMN IF NOT EXISTS last_scan_imported integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_scan_skipped integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_scan_failed integer NOT NULL DEFAULT 0;
