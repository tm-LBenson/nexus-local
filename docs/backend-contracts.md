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

## Provider Package

Location: `services/api/internal/providers`

Provider contracts describe replaceable infrastructure:

- `ObjectStore`: MinIO, S3, or another S3-compatible service.
- `VectorIndex`: Qdrant first, pgvector or another index later.
- `JobQueue`: NATS JetStream first, another durable queue later.
- `ModelGateway`: OpenAI-compatible model endpoint backed by local GPU, rented GPU, or hosted API.
- `ModelRouter`: product target name to model/provider route.

Provider implementations can be swapped by deployment profile. Product code should depend on these contracts, not on concrete cloud SDKs.

## Repository Contracts

Location: `services/api/internal/store`

Repository contracts describe product persistence needs before choosing a database implementation. The current `memory` repository exists for local development and fast contract tests. The next production adapter should implement the same interfaces on Postgres.

## First Workflow

`POST /v1/documents/register` creates a document record and queues a `document_ingestion` job. This is deliberately metadata-only for now; real upload will add object storage and content hashing before registration.

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

## TDD Boundary

Tests should come first when changing:

- state machines
- role permissions
- provider contracts
- API request/response shape
- tenant access rules
- data migration rules

Fast-moving UI and exploratory admin workflow can stay lighter until behavior stabilizes.
