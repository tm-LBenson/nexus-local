# Phase 2: Data Sources and Ingestion

## Outcome

A customer can point Nexus Local at a folder, synced drive, export, network share, or future connector and see that source become a maintained knowledge base.

## Why This Matters

Real customers will not upload one file at a time. They will have OneDrive, Teams/SharePoint sync folders, exported tickets, policy directories, case archives, runbooks, spreadsheets, and mixed technical artifacts. The app needs to manage sources, not just files.

## Scope

- Data source domain model.
- Folder import UI and background source scan jobs.
- Include/exclude rules.
- Supported file summary and unsupported file reporting.
- Incremental scan detection.
- Per-source ingestion progress and failure review.
- Source-level delete/reindex controls.

## Implementation Slices

| Slice | Build | Gate |
| --- | --- | --- |
| 2.1 | Data source records with type, name, root/path, status, last scan, and owner. | Source CRUD is tenant-scoped and audited. |
| 2.2 | Import folder UI that creates a source scan job. | User can queue a source scan from the app and see it in Activity. |
| 2.3 | Source scan worker that walks allowed files and creates document jobs. | Fixture folder imports without blocking the API. |
| 2.4 | Default include/exclude safety for supported extensions, hidden/cache paths, symlinks, and max size. | Test folder shows expected accepted/rejected counts and the UI shows scan totals. |
| 2.5 | Hash-based incremental scan with unchanged skips, changed-file replacement, and missing-file cleanup. | Re-scan skips unchanged files, replaces changed files, and deletes documents for missing source paths. |
| 2.6 | Source detail panel with failures, retries, scan totals, recent jobs, and per-file scan outcomes. | User can answer "what happened to this import?" from the UI. |
| 2.7 | Source management controls for edit, refresh, rescan, and archive. | User can correct source paths and manage scans from Library without scripts. |
| 2.8 | Explicit destructive source cleanup. | User can archive a source and delete documents imported from that source, with audit trail and active-scan protection. |
| 2.9 | Source-level reindex controls. | User can queue fresh ingestion jobs for documents from a source without rescanning the folder. |
| 2.10 | Source include/exclude configuration. | User can narrow managed imports with source-relative patterns and see filtered files in scan outcomes. |
| 2.11 | Scheduled source maintenance. | Worker queues due source scans from per-source intervals without browser involvement. |
| 2.12 | Larger import observability. | Source detail shows latest-scan counts, reason buckets, and filterable file outcomes. |
| 2.13 | Paged scan review and CSV export. | Large imports can be reviewed by outcome without loading every scan entry into the browser. |
| 2.14 | First-run source mounting guidance. | Operators can mount a document root into the worker and users see worker-path examples in Library. |
| 2.15 | Connector-oriented source templates. | Users can start from common source shapes such as SharePoint sync, OneDrive sync, network shares, ticket exports, knowledge base exports, and runbooks. |
| 2.16 | Source scan recovery actions. | Failed scans surface direct actions to show failed files, export a failed-only CSV, and retry the scan after fixing the source. |
| 2.17 | Live source scan progress. | An active scan shows current file-result counts as entries arrive, plus a refresh affordance for long imports. |
| 2.18 | Source-level failed document retry. | Users can retry failed document-ingestion jobs for a source without manually opening each failed document. |
| 2.19 | Configurable document ingestion concurrency. | Operators can tune parallel ingestion for larger imports while job claiming stays atomic and tested. |
| 2.20 | Managed source stress gate. | Operators can generate a repeatable mixed source fixture, scan it through the real worker path, verify import/skip/failure counts, wait for ingestion, and prove search works. |
| 2.21 | Richer batch import reporting UI. | Source detail shows a compact scan report with outcome mix, clickable count filters, reason buckets, and faster CSV/review controls. |
| 2.22 | Customer-shaped stress fixtures. | The source stress gate can generate support-ops and governance-style folder trees across supported text/export/transcript formats. |
| 2.23 | Source path preflight jobs. | Users can check that the worker can open/read a mounted source path before starting a scan, and failures show as source activity with clear mount/permission errors. |
| 2.24 | Preflight-guided source onboarding. | Adding a source automatically starts a path check, failed current checks block scans, and the UI offers a direct check-again recovery path. |
| 2.25 | Source import dry-run planning. | Users can queue a worker-side import preview that estimates matched files, skipped files, failures, sample paths, and ingest size before scanning. |

## Dependencies

- Phase 1 loading and job visibility improvements.
- Existing folder import script can inform behavior, but app-owned source jobs should become the main path.
- Object storage and vector cleanup must stay consistent during source reindex/delete.

## Release Gate

- Unit tests for source domain and repository contracts.
- Worker tests for scan behavior.
- Fixture import smoke with nested folders and unsupported files.
- Browser check for source creation, scan progress, and failure review.

## First Build Candidate

Slices 2.1 through 2.25 establish the basic managed-source pipeline, source-level observability, unchanged-file scan skips, changed-file replacement, missing-file cleanup, app-owned source management, explicit destructive cleanup, manual reindexing, configurable source filters, scheduled source scans, latest-scan outcome review, paged scan export, first-run source mounting guidance, connector-oriented source templates, source scan recovery actions, live source scan progress, source-level failed document retry, configurable document ingestion concurrency, a repeatable managed-source stress gate, richer batch import reporting in the UI, customer-shaped stress fixtures, worker-context source path preflight checks, preflight-guided source onboarding, and worker-side source import dry-run planning. Next, turn the dry-run summary into a safer first-scan path by prompting users when an import is large or has many skipped/failed files.
