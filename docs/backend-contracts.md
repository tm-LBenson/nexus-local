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

The domain package should not import Postgres, Qdrant, MinIO, NATS, HTTP clients, or framework code. It owns product rules such as document lifecycle transitions and job state transitions.

## Auth Boundary

Location: `services/api/internal/auth`

HTTP routes use an authenticated principal and then check tenant permissions against memberships. The first implementation supports:

- `AUTH_MODE=dev`: local development fallback principal from `DEV_USER_ID`.
- `AUTH_MODE=trusted-header`: the API trusts user headers set by a fronting identity-aware proxy, then enforces memberships in the repository.

Product services still receive explicit user and tenant IDs, but HTTP handlers now derive owner IDs from the authenticated principal instead of trusting client-provided owner fields.

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

`POST /v1/documents/register` creates a document record and queues a `document_ingestion` job from already-known object metadata.

`POST /v1/documents/upload` accepts a multipart file, stores it through the configured `ObjectStore`, creates the document record, and queues a `document_ingestion` job.

`DELETE /v1/documents/<document_id>?tenant_id=<tenant_id>` soft-deletes a document, removes its stored object when object storage is configured, and removes vectors when a vector index is configured. Normal document lists omit deleted documents.

`GET /v1/jobs?tenant_id=<tenant_id>` lists recent tenant activity for uploaded documents and background work. Results are newest-updated first, default to 25 jobs, and cap at 100.

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

Jobs may carry `resource_type` and `resource_id` so a worker can claim work without needing to infer the subject from side effects. Document ingestion jobs use `document/<document_id>`.

## TDD Boundary

Tests should come first when changing:

- state machines
- role permissions
- provider contracts
- API request/response shape
- tenant access rules
- data migration rules

Fast-moving UI and exploratory admin workflow can stay lighter until behavior stabilizes.
