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

## Dependencies

- Current document upload and worker ingestion.
- Current conversation/search API.
- Current audit event foundation.
- Docker Compose profiles and guided launcher.

## Release Gate

- `go test ./...`
- `npm run build`
- `.\scripts\dev-check.ps1 -Smoke`
- Manual browser check for setup, dashboard, library, activity, settings.

## First Build Candidate

Start with Slice 1.1. The user already observed slow one-token-at-a-time responses and weak loading states. Improving this gives immediate value and makes future testing less confusing.

