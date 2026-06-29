# Backend Contracts

The backend starts with domain rules and provider boundaries before database tables or cloud-specific integrations.

## Domain Package

Location: `services/api/internal/domain`

Stable product concepts:

- tenants
- users
- memberships
- roles and permissions
- conversations
- messages
- documents
- jobs
- audit events

The domain package should not import Postgres, Qdrant, MinIO, NATS, HTTP clients, or framework code. It owns product rules such as document lifecycle transitions and job state transitions.

## Auth Boundary

Location: `services/api/internal/auth`

HTTP routes use an authenticated principal and then check tenant permissions against memberships. The first implementation supports:

- `AUTH_MODE=dev`: local development fallback principal from `DEV_USER_ID`.
- `AUTH_MODE=trusted-header`: the API trusts user headers set by a fronting identity-aware proxy, then enforces memberships in the repository.

Product services still receive explicit user and tenant IDs, but HTTP handlers now derive owner IDs from the authenticated principal instead of trusting client-provided owner fields.

The production auth Compose profile runs this mode behind Caddy. Caddy performs forward-auth, strips spoofable identity headers from incoming requests, and only forwards `X-User-ID` and `X-User-Email` after the auth gateway returns authenticated `Remote-User` and `Remote-Email` headers.

Identity bootstrap endpoints:

- `GET /v1/me`: persists/returns the authenticated principal and tenant memberships.
- `POST /v1/tenants`: creates a generated tenant and grants the authenticated principal `owner`.
- `GET /v1/tenants/<tenant_id>/members`: lists workspace members for owner/admin users.
- `POST /v1/tenants/<tenant_id>/members`: creates or updates a member user and role for owner/admin users.
- `DELETE /v1/tenants/<tenant_id>/members/<user_id>`: removes a workspace member while protecting the last owner.

## Provider Package

Location: `services/api/internal/providers`

Provider contracts describe replaceable infrastructure:

- `ObjectStore`: MinIO, S3, or another S3-compatible service.
- `VectorIndex`: Qdrant first, pgvector or another index later.
- `JobQueue`: NATS JetStream first, another durable queue later.
- `ModelGateway`: OpenAI-compatible model endpoint backed by local GPU, rented GPU, or hosted API.
- `ModelRouter`: product target name to model/provider route.

Provider implementations can be swapped by deployment profile. Product code should depend on these contracts, not on concrete cloud SDKs.

The first model adapter is `providers/openaicompat`, which targets vLLM, LiteLLM, Ollama-compatible OpenAI routes, rented GPU endpoints, or hosted OpenAI-compatible APIs.

## Repository Contracts

Location: `services/api/internal/store`

Repository contracts describe product persistence needs before choosing a database implementation. The current `memory` repository exists for local development and fast contract tests. The next production adapter should implement the same interfaces on Postgres.

Implemented repository adapters:

- `store/memory`: fast local development and unit tests.
- `store/postgres`: production-shaped relational persistence with embedded SQL migrations.

Startup selects the adapter with `PERSISTENCE_BACKEND`. `RUN_MIGRATIONS=true` applies embedded migrations on boot, which is useful for Compose/self-host installs. Larger production deployments may move this into a dedicated migration job later.

## First Workflow

`POST /v1/documents/register` creates a document record and queues a `document_ingestion` job from already-known object metadata. Unsupported document names are rejected with `415 Unsupported Media Type`.

`POST /v1/documents/upload` accepts a multipart file, validates its filename/content type, stores it through the configured `ObjectStore`, creates the document record, and queues a `document_ingestion` job. Unsupported files are rejected with `415 Unsupported Media Type` before storage.

`DELETE /v1/documents/<document_id>?tenant_id=<tenant_id>` soft-deletes a document, removes its stored object when object storage is configured, and removes vectors when a vector index is configured. Normal document lists omit deleted documents.

`GET /v1/data-sources?tenant_id=<tenant_id>` lists managed knowledge sources for a tenant. `GET /v1/data-sources/<source_id>?tenant_id=<tenant_id>` returns the source plus recent related source-scan jobs, a `scan_summary` for the newest scan job represented in the recent entries, `scan_entries_page` metadata, and paged `scan_entries` with file path, outcome, reason, message, size, content hash, and imported document ID when available. Source detail accepts `scan_entry_limit`, `scan_entry_offset`, and `scan_entry_outcome` (`imported`, `skipped`, `failed`, or `deleted`). `GET /v1/data-sources/<source_id>/scan-entries.csv?tenant_id=<tenant_id>` exports scan entries for review and accepts optional `outcome`. `POST /v1/data-sources` creates a source record with type, name, root path, include patterns, exclude patterns, optional scan interval, owner, status, timestamps, next scan due time, and last-scan imported/skipped/failed counts. `PATCH /v1/data-sources/<source_id>` updates source metadata, source include/exclude patterns, and scheduled scan interval. `POST /v1/data-sources/<source_id>/scan?tenant_id=<tenant_id>` queues a `source_scan` job for that source and rejects duplicate active scans. The worker queues scheduled scans when `next_scan_at` is due, then walks the source path from its own machine/container, applies source-relative include/exclude patterns, uploads supported files, skips unsupported or unsafe files, skips unchanged files by content hash, replaces the prior document for changed source paths, deletes prior documents for missing or newly filtered source paths after a clean scan, records per-file outcomes, and queues normal `document_ingestion` jobs for imported files. `POST /v1/data-sources/<source_id>/reindex?tenant_id=<tenant_id>` queues fresh `document_ingestion` jobs for currently active documents referenced by the latest source scan entries, skips deleted, missing, and already-active documents, and rejects archived sources or active source scans. `DELETE /v1/data-sources/<source_id>?tenant_id=<tenant_id>` archives the source record without deleting documents. `DELETE /v1/data-sources/<source_id>?tenant_id=<tenant_id>&delete_documents=true` archives the source and deletes currently active documents referenced by the latest source scan entries; it rejects active source scans. This is the contract folder, synced drive, network share, export, and connector scan workers attach to.

`GET /v1/jobs?tenant_id=<tenant_id>` lists recent tenant activity for uploaded documents and background work. Results are newest-updated first, default to 25 jobs, and cap at 100. Failed jobs include `error_message` with the ingestion or worker failure reason.

`GET /v1/audit-events?tenant_id=<tenant_id>` lists recent tenant audit events for administrative review. Results are newest first, default to 50 events, and cap at 200. Events include actor, action, resource type, resource ID, outcome, metadata, and creation time.

Current audited actions:

- `document.uploaded`
- `data_source.created`
- `data_source.scan_requested`
- `data_source.reindex_requested`
- `data_source.updated`
- `data_source.archived`
- `data_source.documents_deleted`
- `search.completed`
- `conversation.ask`

`GET /v1/conversations?tenant_id=<tenant_id>` lists recent tenant conversations, and `GET /v1/conversations/<conversation_id>/messages?tenant_id=<tenant_id>` reads the ordered transcript for resume/review flows.

`POST /v1/model-targets/check` routes a configured model target and performs a short non-persistent completion to verify first-run model gateway connectivity. It returns the resolved route, model, finish reason, and latency when the gateway responds.

The first worker can claim a document ingestion job and advance the document/job state flow. Extraction, chunking, embeddings, and vector writes are the next layer.

## Current State Machines

Job states:

```text
queued -> running | canceled
running -> succeeded | failed | retrying | canceled
retrying -> queued | running | failed | canceled
```

Document states:

```text
uploaded -> processing | failed | deleted
processing -> ready | failed | deleted
ready -> deleted
failed -> processing | deleted
```

Terminal states should stay terminal unless we intentionally add a recovery workflow.

Jobs may carry `resource_type` and `resource_id` so a worker can claim work without needing to infer the subject from side effects. Document ingestion jobs use `document/<document_id>`. Failed jobs should carry a concise `error_message`; retrying creates a fresh queued job and clears any stale error when it is claimed. `WORKER_DOCUMENT_CONCURRENCY` allows one worker process to claim multiple document ingestion jobs in parallel; repository job claiming must remain atomic so no queued job is processed twice.

## TDD Boundary

Tests should come first when changing:

- state machines
- role permissions
- provider contracts
- API request/response shape
- tenant access rules
- data migration rules

Fast-moving UI and exploratory admin workflow can stay lighter until behavior stabilizes.
