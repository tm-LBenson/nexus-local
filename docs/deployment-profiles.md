# Deployment Profiles

Run `scripts\setup.ps1` to generate `.env` for any Compose profile before starting the stack. See [Guided Setup](setup.md) for profile-specific examples.

## cpu-lite

Runs the app without a local GPU. Useful for NAS-only installs, demos, admin work, document management, and remote model providers.

Core services:

- API
- Worker
- Web
- Postgres
- MinIO
- Qdrant
- NATS
- Valkey

The Compose CPU profile uses Postgres persistence by default and runs embedded migrations at API startup.
Use [Backup and Restore](backup-restore.md) before removing volumes or upgrading the stack.

## split-nas-gpu

Recommended for your current hardware.

NAS:

- API
- Worker
- Web
- Postgres
- MinIO
- Qdrant
- NATS
- Valkey
- Backups

Postgres data should live on the NAS volume set, with regular backups. The desktop GPU side should be treated as replaceable compute, not the source of truth.
The basic backup script covers the NAS-side durable stores: Postgres, MinIO, and Qdrant.

Desktop:

- vLLM or Ollama
- Embedding service if GPU accelerated
- Heavy ingestion workers if needed

The app talks to the desktop through `MODEL_GATEWAY_BASE_URL`.

Use the `starter` provider preset when the desktop only exposes chat completions. Use the `semantic` preset when you also run an OpenAI-compatible embedding service and can set `EMBEDDING_BASE_URL`.

## gpu-local

One GPU machine runs the whole stack. Good for a lab box, power user desktop, or customer-owned appliance with a large GPU.

## prod-single-node

One production server with fast NVMe, ECC memory if possible, and a 24 GB or larger GPU. Docker Compose remains acceptable here if operations stay simple.

## prod-auth

Production-facing Compose profile with Caddy as the public entrypoint and the API running in `AUTH_MODE=trusted-header`.

Use:

```powershell
docker compose -f deploy\compose\compose.prod-auth.yml --env-file .env up -d --build
```

This profile expects an Authelia-compatible forward-auth gateway at `AUTHELIA_INTERNAL_URL`. Caddy strips client-supplied identity headers, asks the auth gateway to authorize the request, maps `Remote-User` and `Remote-Email` into the trusted API headers, and proxies frontend API calls through `/api`.

See [Production Auth](production-auth.md) for setup and security notes.
See [Backup and Restore](backup-restore.md) for the first operational backup routine.

## prod-k3s

K3s or Kubernetes profile for many users, multiple nodes, GPU scheduling, and clean upgrades. This is the right target once deployments need orchestration rather than just containers.

## external-ai

The application is self-hosted, but model inference is remote. This can point at AWS GPU, a rented inference endpoint, OpenAI-compatible APIs, or a customer-managed model server.
