# Nexus Local

Nexus Local is a self-hosted AI workspace for private document ingestion, semantic search, and retrieval-augmented chat. It is designed to run on local hardware, a NAS plus GPU workstation, rented GPU infrastructure, or a conventional cloud server without tying the application to one vendor.

The project is early, but the core shape is already in place: a replaceable backend, a slim web UI, provider-neutral storage/search/model interfaces, and Docker profiles for self-hosted deployment.

## What It Does

- Upload documents into tenant-scoped workspaces.
- Manage workspace members and roles for self-hosted team access.
- Extract text from UTF-8 text files, PDFs, and OpenXML Office files.
- Chunk, embed, and index documents for semantic retrieval.
- Run optional self-hosted TEI embeddings for real semantic search.
- Search across indexed document chunks.
- Ask questions over uploaded documents through an OpenAI-compatible model gateway.
- Track ingestion jobs and background activity.
- Review and resume conversation history.
- Soft-delete documents and clean up stored objects and vectors.

Supported upload formats are UTF-8 text-like files (`.txt`, `.md`, `.json`, `.html`, `.htm`, `.csv`, `.tsv`, `.vtt`), PDFs, and OpenXML Office files (`.docx`, `.pptx`, `.xlsx`). Unsupported files are rejected before they enter the ingestion queue.

## Architecture

Nexus Local is built as a small monorepo:

```text
apps/web              React/Vite frontend
services/api          Go HTTP API and worker binaries
deploy/compose        Docker Compose deployment profiles
docs                  Architecture and operating notes
scripts               Local helper scripts
```

The backend keeps infrastructure replaceable through provider contracts:

- Relational data: memory for fast local development, Postgres for durable deployments.
- Object storage: memory for local development, MinIO/S3-compatible storage for self-hosted deployments.
- Vector search: memory for tests and single-process experiments, Qdrant for shared deployments.
- Inference: OpenAI-compatible model gateway for local vLLM/Ollama-compatible services, rented GPUs, or hosted providers.
- Workers: Go worker process for asynchronous document ingestion.

## Requirements

For local development:

- Go 1.25+
- Node.js 22+
- npm
- PowerShell 7+ (`pwsh`) on Linux/macOS if using the launcher or helper scripts

For the containerized stack:

- Docker
- Docker Compose

For useful AI answers beyond mock/local hashing:

- An OpenAI-compatible chat endpoint
- Optional external embedding endpoint, or the built-in hash embedder for development/testing

For hardware planning, see [Deployment Scenarios](docs/deployment-scenarios.md). Short version:

| Scenario | Practical floor | Good test target |
| --- | --- | --- |
| App only, remote AI | 4 CPU cores, 16 GB RAM, 50 GB SSD, no GPU | 4-8 CPU cores, 32 GB RAM, 100 GB SSD |
| Local AI test box | 6-8 CPU cores, 32 GB RAM, NVIDIA GPU with 8 GB VRAM | 8+ CPU cores, 64 GB RAM, NVIDIA GPU with 16 GB+ VRAM |
| Team/on-prem node | 8+ CPU cores, 64 GB RAM, fast NVMe, remote or local GPU | 16+ CPU cores, 128 GB RAM, NVMe, 24 GB+ GPU or rented GPU endpoint |

Live Postgres and Qdrant data should use Docker named volumes on local SSD/NVMe storage when possible. Use large HDDs, NAS shares, or object storage for uploaded document archives, model caches, and backups rather than hot database/vector storage.

## Quick Start

Clone the repository:

```powershell
git clone https://github.com/tm-LBenson/nexus-local.git
cd nexus-local
```

For a guided local run, start the launcher.

Windows:

```powershell
.\nexus.ps1
```

Linux/macOS:

```bash
./nexus
```

The launcher provides a small terminal menu for setup, start/stop, opening the web UI, health checks, smoke tests, backups, and restores. The same entry point also supports direct commands:

Windows:

```powershell
.\nexus.ps1 setup -Profile cpu-lite -ProviderPreset starter -Force
.\nexus.ps1 up
.\nexus.ps1 smoke-no-ask
.\nexus.ps1 down
```

Linux/macOS:

```bash
./nexus setup -Profile cpu-lite -ProviderPreset starter -Force
./nexus up
./nexus smoke-no-ask
./nexus down
```

You can still run the underlying scripts directly. Start the full local stack:

```powershell
.\scripts\setup.ps1 -Profile cpu-lite
.\scripts\dev-up.ps1
```

Check the stack:

```powershell
.\scripts\dev-check.ps1
```

Run an end-to-end smoke test:

```powershell
.\scripts\dev-check.ps1 -Smoke
```

The smoke test creates a temporary workspace, uploads a checked-in fixture document, waits for worker ingestion, searches the indexed chunks, asks a question through the configured model gateway, verifies conversation history, and deletes the temporary document. The workspace remains until workspace deletion exists; pass `-TenantId` to reuse an existing workspace. Use `-SkipAsk` for an ingestion/search-only check when no OpenAI-compatible model gateway is configured.

Stop the stack:

```powershell
.\scripts\dev-down.ps1
```

Use `.\scripts\dev-down.ps1 -Volumes` when you want to remove local Postgres, MinIO, Qdrant, NATS, and Valkey data volumes too.
Run `.\scripts\backup.ps1` before removing volumes or doing destructive maintenance.

For a first Windows Docker Desktop test run without a configured model gateway, use:

```powershell
.\scripts\setup.ps1 -Profile cpu-lite -ProviderPreset starter -Force
.\scripts\dev-up.ps1
.\scripts\dev-check.ps1 -Smoke -SkipAsk
```

To test with a local OpenAI-compatible model server running on the Windows host, point containers at `host.docker.internal`:

```powershell
.\scripts\setup.ps1 `
  -Profile cpu-lite `
  -ProviderPreset starter `
  -ModelGatewayBaseUrl "http://host.docker.internal:11434/v1" `
  -Force

.\scripts\dev-up.ps1
.\scripts\dev-check.ps1 -Smoke
```

See [Guided Setup](docs/setup.md) for split NAS/GPU, local GPU, and production auth profile generation.
See [Deployment Scenarios](docs/deployment-scenarios.md) for Windows Docker Desktop, NAS plus GPU desktop, on-prem, Coolify/Linux, cloud, rented GPU, minimum requirements, and storage guidance.
See [Backup and Restore](docs/backup-restore.md) for basic Postgres, MinIO, and Qdrant snapshots.

## Local Development

Run only the API with in-memory storage:

```powershell
cd services\api
go test ./...
go run ./cmd/api
```

In another terminal, run the web app:

```powershell
cd apps\web
npm install
npm run dev
```

Open the Vite URL printed by the web dev server, usually:

```text
http://localhost:5173
```

## Docker Compose Details

The CPU profile starts the API, web app, worker, Postgres, MinIO, Qdrant, NATS, and Valkey:

```powershell
cd deploy\compose
docker compose -f compose.cpu.yml up --build
```

Default service URLs:

- Web: `http://localhost:5173`
- API health: `http://localhost:8080/healthz`
- API readiness: `http://localhost:8080/readyz`
- MinIO console: `http://localhost:9001`
- Qdrant: `http://localhost:6333`

## Configuration

The API is configured through environment variables. Common settings:

| Variable | Default | Purpose |
| --- | --- | --- |
| `AUTH_MODE` | `dev` | `dev` for local development, `trusted-header` behind an identity-aware proxy. |
| `PERSISTENCE_BACKEND` | `memory` | `memory` or `postgres`. |
| `RUN_MIGRATIONS` | `false` | Runs embedded Postgres migrations on startup. |
| `OBJECT_STORAGE_BACKEND` | `memory` | `memory` or `minio`. |
| `VECTOR_BACKEND` | `memory` | `memory` or `qdrant`. |
| `EMBEDDING_RUNTIME` | `none` | `none`, `external`, `cpu`, or `gpu`. |
| `EMBEDDING_BACKEND` | `hash` | `hash` or OpenAI-compatible embeddings. |
| `EMBEDDING_BASE_URL` | `http://localhost:8082/v1` | OpenAI-compatible embedding gateway when semantic embeddings are enabled. |
| `EMBEDDING_GATEWAY_IMAGE` | `ghcr.io/huggingface/text-embeddings-inference:cpu-1.9` | TEI image for self-hosted embeddings. |
| `PROVIDER_PRESET` | `starter` | UI/setup label: `starter` for hash embeddings, `semantic` for OpenAI-compatible embeddings. |
| `MODEL_GATEWAY_BASE_URL` | `http://localhost:8000/v1` | OpenAI-compatible chat gateway. |
| `GENERAL_MODEL_ID` | `Qwen/Qwen2.5-7B-Instruct` | Default model ID sent to the gateway. |

See [.env.example](.env.example) and [Deployment Profiles](docs/deployment-profiles.md) for more deployment-oriented settings.

## API Surface

Useful endpoints during development:

- `GET /healthz`
- `GET /readyz`
- `GET /v1/me`
- `POST /v1/tenants`
- `GET /v1/tenants/{tenant_id}/members`
- `POST /v1/tenants/{tenant_id}/members`
- `DELETE /v1/tenants/{tenant_id}/members/{user_id}`
- `GET /v1/model-targets`
- `POST /v1/model-targets/check`
- `GET /v1/documents?tenant_id=tenant_1`
- `GET /v1/documents/{document_id}?tenant_id=tenant_1`
- `GET /v1/documents/{document_id}/download?tenant_id=tenant_1`
- `POST /v1/documents/upload`
- `POST /v1/documents/{document_id}/retry?tenant_id=tenant_1`
- `DELETE /v1/documents/{document_id}?tenant_id=tenant_1`
- `GET /v1/jobs?tenant_id=tenant_1`
- `POST /v1/search`
- `POST /v1/conversations/ask`
- `POST /v1/conversations/ask/stream`
- `GET /v1/conversations?tenant_id=tenant_1`
- `GET /v1/conversations/{conversation_id}/messages?tenant_id=tenant_1`
- `DELETE /v1/conversations/{conversation_id}?tenant_id=tenant_1`

The stream endpoint returns server-sent events: `status`, `delta`, `error`, and `done`.
Search and ask responses include a `source` object for each hit with the document name, document ID, chunk ID, and chunk index when available.
Search and ask requests can include `document_id` to scope retrieval to one uploaded document.
Job responses include `error_message` when ingestion fails, and failed documents can be retried from the document detail view or retry endpoint.

Example document upload:

```powershell
Invoke-RestMethod http://localhost:8080/v1/documents/upload `
  -Method Post `
  -Form @{ tenant_id = 'tenant_1'; file = Get-Item .\README.md }
```

Example member add:

```powershell
Invoke-RestMethod http://localhost:8080/v1/tenants/tenant_1/members `
  -Method Post `
  -ContentType 'application/json' `
  -Body '{"user_id":"user_2","email":"user2@example.local","role":"member"}'
```

Example search:

```powershell
Invoke-RestMethod http://localhost:8080/v1/search `
  -Method Post `
  -ContentType 'application/json' `
  -Body '{"tenant_id":"tenant_1","query":"deployment notes","limit":5}'
```

Example smoke test:

```powershell
.\scripts\dev-smoke.ps1
.\scripts\dev-smoke.ps1 -SkipAsk
```

Example question:

```powershell
Invoke-RestMethod http://localhost:8080/v1/conversations/ask `
  -Method Post `
  -ContentType 'application/json' `
  -Body '{"tenant_id":"tenant_1","question":"What do the deployment notes say?","limit":5}'
```

## Tests

Run backend tests:

```powershell
cd services\api
go test ./...
```

Run frontend build checks:

```powershell
cd apps\web
npm run build
```

Postgres integration tests are opt-in:

```powershell
$env:TEST_DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable'
cd services\api
go test ./internal/store/postgres
```

Qdrant integration tests are opt-in:

```powershell
$env:TEST_QDRANT_URL='http://localhost:6333'
cd services\api
go test ./internal/providers/vector/qdrant
```

## Deployment Profiles

Setup profiles and deployment directions are documented in [Deployment Profiles](docs/deployment-profiles.md), with scenario guidance in [Deployment Scenarios](docs/deployment-scenarios.md):

| Name | Kind | Use it for |
| --- | --- | --- |
| `cpu-lite` | setup profile | Windows Docker Desktop tests, NAS-only app services, CPU-only servers, or remote AI providers. |
| `split-nas-gpu` | setup profile | NAS/Linux/cloud app services with inference on a desktop GPU, laptop GPU, or rented GPU endpoint. |
| `gpu-local` | setup profile | One NVIDIA GPU machine running the full stack plus the bundled vLLM-compatible gateway. |
| `prod-auth` | setup profile | Production-facing Compose deployment with Caddy and trusted-header auth behind forward auth. |
| `prod-single-node` | direction | One on-prem or cloud server running the application stack. |
| `prod-k3s` | direction | Future Kubernetes direction for many users, multiple nodes, and GPU scheduling. |
| `external-ai` | pattern | Self-hosted app/data layer with hosted, cloud, or customer-managed model endpoints. |

See [Production Auth](docs/production-auth.md) for the Caddy/trusted-header deployment profile.
See [Backup and Restore](docs/backup-restore.md) before changing persistent volumes.

## Roadmap

Near-term priorities:

- Stronger local setup scripts.
- Better worker visibility and retry controls.
- Streaming chat responses.
- More document management tools.
- Observability stack examples with Prometheus/Grafana/OpenTelemetry.

## License

License information has not been finalized yet.
