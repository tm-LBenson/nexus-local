# Nexus Local

This is the new portable foundation for a self-hosted AI application built for private local AI workflows, but designed so the backend, model runtime, storage, vector search, and deployment target can change independently.

## Stack Direction

- Backend: Go API and workers for low-latency I/O, streaming, jobs, and predictable deployment.
- Frontend: React + Vite app shell, kept API-driven and replaceable.
- Data: Postgres for relational state, Qdrant for vector search at scale.
- Storage: S3-compatible object storage through MinIO locally and S3-compatible providers elsewhere.
- Jobs/events: NATS JetStream for durable work queues and event streams.
- Cache/rate limits: Valkey.
- Inference: OpenAI-compatible model gateway, backed by local vLLM/Ollama, rented GPU, or external API.
- Auth: provider-neutral API auth boundary with local `dev` mode and `trusted-header` mode for reverse-proxy/OIDC setups.
- Observability: OpenTelemetry-first, with Prometheus/Grafana/Langfuse planned as separate deployment services.

## TDD Stance

TDD is worth it here, but only where it protects architecture decisions. We will use contract-first tests around provider boundaries, routing, auth, storage, jobs, and data access. We will not slow down early UI exploration with brittle tests for every visual detail.

The first test surface is model routing: the app should not care whether a request goes to a desktop GPU, NAS CPU profile, AWS GPU worker, or hosted provider.

## Quick Start

The API now targets Go 1.25 because the Postgres adapter uses current `pgx`.

Memory-backed local API:

```powershell
cd C:\path\to\nexus-local\services\api
go test ./...
go run ./cmd/api
```

Containerized Postgres-backed stack:

```powershell
cd C:\path\to\nexus-local\deploy\compose
docker compose -f compose.cpu.yml up --build
```

The API image includes two entrypoints:

- `/api`: HTTP API
- `/worker`: background document ingestion worker

Local development defaults to `AUTH_MODE=dev`, which injects `DEV_USER_ID=user_1`. For a production self-hosted deployment, put the API behind a trusted identity-aware proxy and set `AUTH_MODE=trusted-header`; tenant permissions are then checked against stored memberships.

Then open:

- Health: `http://localhost:8080/healthz`
- Readiness: `http://localhost:8080/readyz`
- Current user: `GET http://localhost:8080/v1/me`
- Create tenant: `POST http://localhost:8080/v1/tenants`
- Model targets: `http://localhost:8080/v1/model-targets`
- Documents: `GET http://localhost:8080/v1/documents?tenant_id=tenant_1`
- Jobs/activity: `GET http://localhost:8080/v1/jobs?tenant_id=tenant_1`
- Document search: `POST http://localhost:8080/v1/search`
- Ask over documents: `POST http://localhost:8080/v1/conversations/ask`
- Conversation history: `GET http://localhost:8080/v1/conversations?tenant_id=tenant_1`

Create your first tenant for the authenticated user:

```powershell
Invoke-RestMethod http://localhost:8080/v1/tenants `
  -Method Post `
  -ContentType 'application/json' `
  -Body '{"name":"Personal Workspace"}'
```

First workflow endpoint:

```powershell
Invoke-RestMethod http://localhost:8080/v1/documents/register `
  -Method Post `
  -ContentType 'application/json' `
  -Body '{"tenant_id":"tenant_1","name":"Handbook.md","storage_key":"tenants/tenant_1/documents/source.md","size_bytes":42}'
```

Object-storage-backed upload endpoint:

```powershell
Invoke-RestMethod http://localhost:8080/v1/documents/upload `
  -Method Post `
  -Form @{ tenant_id = 'tenant_1'; file = Get-Item .\README.md }
```

Ingestion extracts text from UTF-8 text files, PDFs, and OpenXML Office files (`.docx`, `.pptx`, `.xlsx`).

Tenant-scoped document search endpoint:

```powershell
Invoke-RestMethod http://localhost:8080/v1/search `
  -Method Post `
  -ContentType 'application/json' `
  -Body '{"tenant_id":"tenant_1","query":"deployment notes","limit":5}'
```

Conversation ask endpoint:

```powershell
Invoke-RestMethod http://localhost:8080/v1/conversations/ask `
  -Method Post `
  -ContentType 'application/json' `
  -Body '{"tenant_id":"tenant_1","question":"What do the deployment notes say?","limit":5}'
```

Postgres integration tests are opt-in so normal test runs stay fast:

```powershell
$env:TEST_DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable'
cd C:\path\to\nexus-local\services\api
go test ./internal/store/postgres
```

Qdrant integration tests are also opt-in:

```powershell
$env:TEST_QDRANT_URL='http://localhost:6333'
cd C:\path\to\nexus-local\services\api
go test ./internal/providers/vector/qdrant
```

## Project Layout

```text
apps/web              React/Vite frontend shell
deploy/compose        Docker Compose deployment profiles
docs                  Architecture and operating notes
scripts               Local helper scripts
services/api          Go API service
```

Key docs:

- [Architecture](docs/architecture.md)
- [Backend contracts](docs/backend-contracts.md)
- [Deployment profiles](docs/deployment-profiles.md)
- [Provider contracts](docs/provider-contracts.md)
- [Testing strategy](docs/testing-strategy.md)
- [Workers](docs/workers.md)

## Deployment Profiles

- `cpu-lite`: NAS-only app services, no local GPU runtime.
- `split-nas-gpu`: NAS runs core services, desktop or rented GPU runs inference.
- `gpu-local`: one GPU machine runs the full stack plus inference.
- `prod-single-node`: one serious server, still Docker based.
- `prod-k3s`: Kubernetes-ready shape for later multi-node installs.
- `external-ai`: app self-hosted, model provider remote.
