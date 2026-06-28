# Deployment Scenarios

This guide maps real hardware and hosting choices to the Nexus Local setup profiles and deployment directions. The goal is to make the same product deployable on a Windows test machine, a NAS plus GPU desktop, a single on-prem server, rented cloud GPUs, or a future clustered install without changing application code.

## Quick Decision Guide

| Scenario | Best fit | Model runtime | Good fit |
| --- | --- | --- | --- |
| Windows Docker Desktop test run | `cpu-lite` | External OpenAI-compatible endpoint or skip ask checks | Fastest first boot on a desktop or laptop. |
| Windows all-in-one GPU lab | `gpu-local` | Bundled vLLM-compatible container | NVIDIA host with enough VRAM for the selected model. |
| CPU-only NAS | `cpu-lite` | Desktop, laptop, rented GPU, or hosted provider | Always-on app/data node without local generation. |
| NAS plus desktop/laptop GPU | `split-nas-gpu` | OpenAI-compatible gateway on GPU machine | Good long-term home lab or small office setup. |
| Small Ubuntu/Coolify server | `cpu-lite` or `prod-auth` | Remote model endpoint | App/data host with inference elsewhere. |
| Single on-prem GPU server | `gpu-local` for lab, `prod-auth` plus external gateway for production-facing use | Local vLLM/Ollama/LiteLLM | Customer-owned appliance or lab server. |
| Cloud app plus rented GPU | `cpu-lite` or `prod-auth` with external AI | GPU instance or hosted model API | Cloud data plane with replaceable inference. |
| Larger team/on-prem deployment | `prod-auth`, later `prod-k3s` | Local GPU pool, rented GPU, or hosted model provider | Multiple users, stronger auth, backups, and upgrade discipline. |

## Minimum Requirements

These are practical targets for Nexus Local, not vendor minimums for Docker itself.

| Tier | CPU | RAM | Storage | GPU | Notes |
| --- | --- | --- | --- | --- | --- |
| App-only floor | 4 cores | 16 GB | 50 GB SSD | None | Runs web, API, worker, Postgres, MinIO, Qdrant, NATS, and Valkey. Use remote AI or `-SkipAsk` smoke tests. |
| App-only comfortable | 4-8 cores | 32 GB | 100 GB SSD/NVMe | None | Better for larger document sets and self-hosted CPU embeddings. |
| Local AI floor | 6-8 cores | 32 GB | 100+ GB SSD/NVMe | NVIDIA 8 GB VRAM | Good for small or quantized models and basic RAG tests. |
| Local AI comfortable | 8+ cores | 64 GB | 200+ GB NVMe | NVIDIA 16 GB+ VRAM | Good for stronger local models, semantic embeddings, and sustained testing. |
| Serious single node | 16+ cores | 128 GB | 500 GB+ NVMe | NVIDIA 24 GB+ VRAM or remote GPU | Better for multi-user on-prem or heavier ingestion. |
| Split app/data node | 4-8 cores | 32-64 GB | SSD/NVMe for DB/vector, HDD/NAS for backups | None | Calls a desktop/laptop/cloud GPU endpoint. |

For Docker Desktop on Windows, use the WSL2 backend. NVIDIA GPU containers require current NVIDIA drivers and a working WSL2 GPU path. For a test run, Docker Desktop is fine. For an always-on server, prefer Linux, a NAS container host, or a small Ubuntu box.

## Storage Rules

- Use Docker named volumes for Postgres, Qdrant, MinIO, NATS, and Valkey unless you have a specific storage reason not to.
- Keep live Postgres and Qdrant data on local SSD/NVMe storage when possible.
- Avoid Windows bind mounts, SMB shares, or slow network shares for hot database/vector data.
- Use large HDDs, NAS shares, and object storage for backups, document archives, exported snapshots, and model caches.
- Run `scripts\backup.ps1` before deleting volumes, testing restore paths, or changing persistent storage.

Qdrant and Postgres care about random IO. A large HDD can store a lot of data, but it is not the best place for hot vector search or database writes.

## Scenario 1: Windows Docker Desktop Test Run

Use this when you want the fastest local test on a Windows desktop or laptop.

Requirements:

- Windows 10/11 supported by Docker Desktop.
- Docker Desktop with WSL2 backend enabled.
- 16 GB RAM minimum, 32 GB or more recommended.
- NVIDIA driver if you will use a local GPU model server.

Start without a model gateway:

```powershell
.\scripts\setup.ps1 -Profile cpu-lite -ProviderPreset starter -Force
.\scripts\dev-up.ps1
.\scripts\dev-check.ps1 -Smoke -SkipAsk
```

Start with a model server running on the Windows host, such as an OpenAI-compatible Ollama, LM Studio, LiteLLM, or vLLM endpoint:

```powershell
.\scripts\setup.ps1 `
  -Profile cpu-lite `
  -ProviderPreset starter `
  -ModelGatewayBaseUrl "http://host.docker.internal:11434/v1" `
  -Force

.\scripts\dev-up.ps1
.\scripts\dev-check.ps1 -Smoke
```

Use `host.docker.internal` when the model server runs on Windows and Nexus Local runs in Docker containers. Use `localhost` only from tools running directly on the Windows host.

## Scenario 2: Windows All-In-One GPU Lab

Use this when the Windows machine should run the app stack and the bundled vLLM-compatible model container.

This path is more demanding than running a native Windows model server. It is best for a GPU with enough VRAM for the configured model. On an 8 GB GPU, use smaller models or prefer Scenario 1 with a native/local model gateway.

```powershell
.\scripts\setup.ps1 `
  -Profile gpu-local `
  -ProviderPreset starter `
  -GeneralModelId "Qwen/Qwen2.5-1.5B-Instruct" `
  -Force

docker compose --env-file .env `
  -f deploy\compose\compose.cpu.yml `
  -f deploy\compose\compose.gpu.yml `
  --profile gpu `
  up -d --build

.\scripts\dev-check.ps1 -Smoke
```

If you enable self-hosted semantic embeddings on GPU, add the embedding overlays by using `-ProviderPreset semantic -EmbeddingRuntime gpu` during setup and the command printed by `setup.ps1`.

## Scenario 3: CPU-Only NAS

Use this when a NAS owns the always-on app and data services but does not have a useful GPU.

```powershell
.\scripts\setup.ps1 `
  -Profile cpu-lite `
  -ProviderPreset starter `
  -ModelGatewayBaseUrl "http://desktop-gpu.local:11434/v1" `
  -Force

docker compose --env-file .env -f deploy\compose\compose.cpu.yml up -d --build
```

This is good for:

- API, web, worker, Postgres, MinIO, Qdrant, NATS, and Valkey.
- Backups and long-lived data.
- Calling a desktop, laptop, cloud GPU, or hosted model provider.

Keep NAS HDDs for archives and backups. Prefer SSD storage for live Postgres and Qdrant volumes if the NAS supports it.

## Scenario 4: NAS Plus Desktop Or Laptop GPU

Use this when the NAS or small server owns data and the GPU machine owns inference. This is the clean split for a desktop with a 3060 Ti, a laptop with a 4090, or a future bigger GPU.

On the NAS/app host:

```powershell
.\scripts\setup.ps1 `
  -Profile split-nas-gpu `
  -ProviderPreset starter `
  -ModelGatewayBaseUrl "http://gpu-machine.local:11434/v1" `
  -Force

docker compose --env-file .env `
  -f deploy\compose\compose.cpu.yml `
  -f deploy\compose\compose.split-nas-gpu.yml `
  up -d --build
```

On the GPU machine:

- Run Ollama, LM Studio, LiteLLM, vLLM, or another OpenAI-compatible gateway.
- Keep the endpoint reachable only on your LAN or private overlay network.
- Use a stable hostname or private IP.

For semantic retrieval with an external embedding service:

```powershell
.\scripts\setup.ps1 `
  -Profile split-nas-gpu `
  -ProviderPreset semantic `
  -EmbeddingRuntime external `
  -ModelGatewayBaseUrl "http://gpu-machine.local:11434/v1" `
  -EmbeddingBaseUrl "http://gpu-machine.local:8082/v1" `
  -EmbeddingModel "BAAI/bge-small-en-v1.5" `
  -EmbeddingDimensions 384 `
  -Force
```

## Scenario 5: Small Ubuntu Or Coolify Server

Use this when the app/data layer runs on a small Linux server and inference runs elsewhere.

For a private test or LAN install:

```powershell
pwsh ./scripts/setup.ps1 `
  -Profile cpu-lite `
  -ProviderPreset starter `
  -ModelGatewayBaseUrl "http://gpu-endpoint.internal:8000/v1" `
  -Force

docker compose --env-file .env -f deploy/compose/compose.cpu.yml up -d --build
```

For a production-facing install, use `prod-auth` and put it behind an Authelia-compatible forward-auth service:

```powershell
pwsh ./scripts/setup.ps1 `
  -Profile prod-auth `
  -PublicUrl "https://nexus.example.com" `
  -AutheliaInternalUrl "http://authelia:9091" `
  -Force

docker compose --env-file .env -f deploy/compose/compose.prod-auth.yml up -d --build
```

Coolify is a good fit for Linux-hosted Compose services. Treat Windows Docker Desktop as a local test target, not as a Coolify deployment target.

## Scenario 6: Cloud App Plus Rented GPU

Use this when the app should run on a conventional cloud server and model inference should run on rented GPU infrastructure.

Recommended shape:

- App/data server: `cpu-lite` or `prod-auth`.
- Model server: rented GPU instance running vLLM, Ollama, LiteLLM, or another OpenAI-compatible gateway.
- Network: private VPC, WireGuard, Tailscale, or strict firewall rules.
- Storage: managed Postgres/S3-compatible storage can be added later because Nexus Local uses provider contracts.

Setup looks like:

```powershell
pwsh ./scripts/setup.ps1 `
  -Profile cpu-lite `
  -ProviderPreset starter `
  -ModelGatewayBaseUrl "https://gpu-gateway.example.com/v1" `
  -ModelGatewayApiKey "replace-me" `
  -Force
```

Use `semantic` with `-EmbeddingRuntime external` if the rented GPU or hosted provider also exposes embeddings.

## Scenario 7: Single On-Prem GPU Server

Use this for a lab box, appliance-style server, or office server with a large NVIDIA GPU.

For lab use:

```powershell
pwsh ./scripts/setup.ps1 -Profile gpu-local -ProviderPreset starter -Force
```

Then run the command printed by setup.

For a production-facing deployment, prefer:

- `prod-auth` for the web/API entrypoint.
- A separate local model gateway reachable over the host or LAN.
- Regular backups with `scripts\backup.ps1`.
- Restore testing with `scripts\restore.ps1`.

The current Compose profiles intentionally keep production auth and bundled GPU inference separate. That makes the public entrypoint simpler and keeps model runtime changes from rewriting the app layer.

## Scenario 8: Larger Team Or Clustered Install

Use this direction when you need many users, several workers, multiple GPUs, or cleaner upgrades.

Target shape:

- `prod-auth` today for authenticated Compose deployments.
- `prod-k3s` direction for Kubernetes or K3s later.
- Postgres, object storage, and Qdrant with documented backup/restore.
- GPU inference as a separate service pool behind an OpenAI-compatible gateway.
- Workers scaled independently from the API.

The app is already structured for this: product code calls provider contracts rather than AWS-only, local-only, or GPU-only APIs.

## Machine Notes

A desktop with an Intel 12900-class CPU, 64 GB RAM, and an RTX 3060 Ti-class GPU is a strong all-in-one test machine. It should run the app stack comfortably and handle small or quantized local models.

A laptop with a 4090-class GPU is a strong inference test machine because of VRAM, even though it may throttle under sustained load. Keep it plugged in, use performance mode, and watch temperatures during long ingestion or generation runs.

A 64 GB NAS without a GPU is a good app/data/storage node. It should not be expected to run local generation well, but it can call a desktop, laptop, rented GPU, or hosted provider.

## Validation Path

For any deployment scenario:

```powershell
.\scripts\dev-check.ps1
.\scripts\dev-check.ps1 -Smoke -SkipAsk
```

When the model gateway is configured and reachable:

```powershell
.\scripts\dev-check.ps1 -Smoke
```

The full smoke gate creates a temporary workspace, uploads a fixture document, waits for ingestion, searches, asks a question, verifies conversation history, and cleans up the temporary document.

## External References

- Docker Desktop Windows requirements: https://docs.docker.com/desktop/setup/install/windows-install/
- Docker Desktop WSL2 backend: https://docs.docker.com/desktop/features/wsl/
- Docker Desktop GPU support: https://docs.docker.com/desktop/features/gpu/
- Ollama GPU support: https://docs.ollama.com/gpu
