# Phase 1: MVP Stabilization

## Outcome

A technical user can install Nexus Local, create a workspace, upload documents, wait for ingestion, search, ask questions, review sources/history/audit events, and recover from common setup or ingestion problems.

## Why This Comes First

The product needs a dependable core loop before broad connectors, governance, or agents matter. If upload, ingestion, search, ask, and audit are not boringly reliable, every later feature inherits that instability.

## Scope

- Setup wizard and launcher reliability.
- Daily dashboard flow for ask, upload, search, and recent activity.
- Clear loading and streaming states for slow local models.
- Ingestion status, retry, and failure handling.
- Basic audit review in Settings.
- Backup/restore verification.
- End-to-end smoke test as a required release gate.

## Implementation Slices

| Slice | Build | Gate |
| --- | --- | --- |
| 1.1 | Streaming answer UX polish with spinner, status text, timeout recovery, and disabled states. | Slow model test shows clear progress and no frozen UI. |
| 1.2 | Dashboard cleanup for active jobs, recent failures, and next action links. | User can find what is stuck from the first screen. |
| 1.3 | Library actions for retry, delete, download, and detail without layout jumps. | Failed and ready documents are manageable from one page. |
| 1.4 | Audit filters by action/outcome/actor/date in Settings. | Admin can find recent sensitive actions without scanning all rows. |
| 1.5 | Backup/restore smoke check in local Docker profile. | Restore instructions are verified against a test backup. |
| 1.6 | First-run sample flow polish. | New user can create a workspace, import a sample, search, and ask without external docs. |
| 1.7 | Workspace deletion and smoke cleanup. | Empty/test workspaces can be deleted, and the smoke gate removes its temporary workspace. |

## Current Status

- Done: Slice 1.7 workspace deletion and smoke cleanup.
- Done: Slice 1.1 ask streaming feedback, timeout handling, and cancel flow.
- Done: Slice 1.2 dashboard attention panel with active work, failures, and next actions.
- Done: Slice 1.3 library action polish for retry/delete/download/detail states.
- Done: Slice 1.4 audit filters by action/outcome/actor/date.
- Done: Slice 1.5 backup/restore smoke check in local Docker profile.
- Done: Slice 1.6 first-run sample flow polish.
- Next: Phase 1 release-candidate review against the full MVP gate.

## Dependencies

- Current document upload and worker ingestion.
- Current conversation/search API.
- Current audit event foundation.
- Docker Compose profiles and guided launcher.

## Release Gate

- `go test ./...`
- `npm run build`
- `.\scripts\dev-check.ps1 -Smoke`
- `.\scripts\dev-check.ps1 -BackupSmoke -BackupRestoreRoundTrip`
- Manual browser check for setup, dashboard, library, activity, settings.

## First Build Candidate

The immediate build priority is the Phase 1 release-candidate review. All listed Phase 1 slices now have implementation coverage, so the next pass should run the full gate, review the fresh-install UI end to end, and turn any remaining gaps into the first MVP release checklist.
