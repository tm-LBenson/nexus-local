# Deployment Profiles

## cpu-lite

Runs the app without a local GPU. Useful for NAS-only installs, demos, admin work, document management, and remote model providers.

Core services:

- API
- Web
- Postgres
- MinIO
- Qdrant
- NATS
- Valkey

The Compose CPU profile uses Postgres persistence by default and runs embedded migrations at API startup.

## split-nas-gpu

Recommended for your current hardware.

NAS:

- API
- Web
- Postgres
- MinIO
- Qdrant
- NATS
- Valkey
- Backups

Postgres data should live on the NAS volume set, with regular backups. The desktop GPU side should be treated as replaceable compute, not the source of truth.

Desktop:

- vLLM or Ollama
- Embedding service if GPU accelerated
- Heavy ingestion workers if needed

The app talks to the desktop through `MODEL_GATEWAY_BASE_URL`.

## gpu-local

One GPU machine runs the whole stack. Good for a lab box, power user desktop, or customer-owned appliance with a large GPU.

## prod-single-node

One production server with fast NVMe, ECC memory if possible, and a 24 GB or larger GPU. Docker Compose remains acceptable here if operations stay simple.

## prod-k3s

K3s or Kubernetes profile for many users, multiple nodes, GPU scheduling, and clean upgrades. This is the right target once deployments need orchestration rather than just containers.

## external-ai

The application is self-hosted, but model inference is remote. This can point at AWS GPU, a rented inference endpoint, OpenAI-compatible APIs, or a customer-managed model server.
