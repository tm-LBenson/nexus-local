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
| 2.3 | Source scan worker that walks allowed files and creates document jobs. | Large fixture folder imports without blocking the API. |
| 2.4 | Include/exclude rules for extensions, glob-like paths, hidden folders, and max size. | Test folder shows expected accepted/rejected counts. |
| 2.5 | Incremental scan with changed/new/deleted detection. | Re-scan updates only changed files and preserves stable records. |
| 2.6 | Source detail page with failures, retries, skipped files, and recent jobs. | User can answer "what happened to this import?" from the UI. |

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

Start with Slice 2.1, then 2.2. The domain contract should come before the UI so every imported file has a traceable source.
