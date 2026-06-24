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
- Observability: OpenTelemetry-first, with Prometheus/Grafana/Langfuse planned as separate deployment services.

## TDD Stance

TDD is worth it here, but only where it protects architecture decisions. We will use contract-first tests around provider boundaries, routing, auth, storage, jobs, and data access. We will not slow down early UI exploration with brittle tests for every visual detail.

The first test surface is model routing: the app should not care whether a request goes to a desktop GPU, NAS CPU profile, AWS GPU worker, or hosted provider.

## Quick Start

```powershell
cd C:\path\to\nexus-local\services\api
go test ./...
go run ./cmd/api
```

Then open:

- Health: `http://localhost:8080/healthz`
- Readiness: `http://localhost:8080/readyz`
- Model targets: `http://localhost:8080/v1/model-targets`

First workflow endpoint:

```powershell
Invoke-RestMethod http://localhost:8080/v1/documents/register `
  -Method Post `
  -ContentType 'application/json' `
  -Body '{"tenant_id":"tenant_1","owner_id":"user_1","name":"Handbook.md","storage_key":"tenants/tenant_1/documents/source.md","size_bytes":42}'
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

## Deployment Profiles

- `cpu-lite`: NAS-only app services, no local GPU runtime.
- `split-nas-gpu`: NAS runs core services, desktop or rented GPU runs inference.
- `gpu-local`: one GPU machine runs the full stack plus inference.
- `prod-single-node`: one serious server, still Docker based.
- `prod-k3s`: Kubernetes-ready shape for later multi-node installs.
- `external-ai`: app self-hosted, model provider remote.
