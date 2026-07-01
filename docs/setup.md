# Guided Setup

Use `scripts/setup.ps1` to generate a deployment-ready `.env` file and validate the selected Compose profile.

For hardware and hosting choices before choosing a profile, see [Deployment Scenarios](deployment-scenarios.md).

```powershell
.\scripts\setup.ps1
```

The script asks for a profile, writes `.env`, checks common host ports, validates Docker Compose when Docker is available, and prints the exact start command.

## Profiles

```powershell
.\scripts\setup.ps1 -Profile cpu-lite
.\scripts\setup.ps1 -Profile split-nas-gpu
.\scripts\setup.ps1 -Profile gpu-local
.\scripts\setup.ps1 -Profile prod-auth
```

Profile behavior:

- `cpu-lite`: NAS or CPU-only install with remote or optional model gateway.
- `split-nas-gpu`: core app on NAS, inference on a desktop or rented GPU endpoint.
- `gpu-local`: full stack plus local vLLM-compatible gateway through the GPU Compose overlay.
- `prod-auth`: Caddy entrypoint with trusted-header auth behind an Authelia-compatible gateway.

## Provider Presets

Profiles decide where containers run. Provider presets decide how model and embedding providers are wired.

- `starter`: OpenAI-compatible chat endpoint with local hash embeddings. Best for first boot, demos, and hardware-light installs.
- `semantic`: OpenAI-compatible chat and OpenAI-compatible embeddings. Best for real retrieval quality.

Interactive setup asks for the preset and embedding runtime. Non-interactive setup defaults to `starter` with no embedding service. When you choose `semantic`, non-interactive setup defaults to a self-hosted CPU embedding service.

```powershell
.\scripts\setup.ps1 -Profile split-nas-gpu -ProviderPreset semantic
```

Embedding runtime options:

- `none`: use hash embeddings; no embedding service.
- `external`: use an existing OpenAI-compatible embedding endpoint.
- `cpu`: start the self-hosted TEI CPU embedding service.
- `gpu`: start the self-hosted TEI GPU embedding service.

## Source Mounts

Managed source scans run in the worker container, so source paths in Library should usually be container paths such as `/sources/primary`.

To mount a readable document root during setup:

```powershell
.\scripts\setup.ps1 `
  -Profile cpu-lite `
  -SourceHostPath "C:\Path\To\Docs" `
  -SourceContainerPath "/sources/primary"
```

When `NEXUS_SOURCE_HOST_PATH` is set, setup and the launcher include `deploy/compose/compose.sources.yml` automatically. See [Source Mounts](source-mounts.md) for OneDrive, Teams/SharePoint sync, NAS, local folder, and export examples.

## Useful Options

Generate without prompts:

```powershell
.\scripts\setup.ps1 -NonInteractive -Profile cpu-lite -Force
```

Generate a self-hosted semantic embedding stack on CPU:

```powershell
.\scripts\setup.ps1 `
  -Profile cpu-lite `
  -ProviderPreset semantic `
  -EmbeddingRuntime cpu
```

Generate a self-hosted semantic embedding stack for an NVIDIA GPU host:

```powershell
.\scripts\setup.ps1 `
  -Profile gpu-local `
  -ProviderPreset semantic `
  -EmbeddingRuntime gpu
```

Set a desktop or rented GPU endpoint:

```powershell
.\scripts\setup.ps1 `
  -Profile split-nas-gpu `
  -ModelGatewayBaseUrl "http://desktop-gpu.local:8000/v1" `
  -ModelGatewayApiKey "local-or-rented-key"
```

Use a separate embedding service for semantic retrieval:

```powershell
.\scripts\setup.ps1 `
  -Profile split-nas-gpu `
  -ProviderPreset semantic `
  -EmbeddingRuntime external `
  -EmbeddingBaseUrl "http://desktop-gpu.local:8082/v1" `
  -EmbeddingModel "BAAI/bge-small-en-v1.5" `
  -EmbeddingDimensions 384
```

Generate production auth config:

```powershell
.\scripts\setup.ps1 `
  -Profile prod-auth `
  -PublicUrl "https://nexus.example.com" `
  -AutheliaInternalUrl "http://authelia:9091" `
  -NexusHttpPort 80 `
  -NexusHttpsPort 443
```

Write somewhere other than `.env`:

```powershell
.\scripts\setup.ps1 -Profile cpu-lite -OutputPath .\.env.local -Force
```

## What It Writes

The generated `.env` includes:

- app/auth mode
- Postgres and MinIO credentials
- object, vector, queue, and cache settings
- provider preset, model gateway URL, API key, and model ID
- embedding runtime, backend, endpoint, key, gateway image, model, and dimensions
- production Caddy/auth settings when selected

Secrets are generated locally as random hex strings unless you pass explicit values.

## Start After Setup

For the default CPU profile:

```powershell
.\scripts\dev-up.ps1
```

`dev-up.ps1` reads `DEPLOYMENT_PROFILE` and optional overlays from the root `.env`, so it can start CPU, split, local GPU, source mount, and embedding profiles without extra flags. The command printed by `setup.ps1` is still useful for seeing the exact Compose files and profiles that will be used.

When the web setup screen is waiting on a model gateway, it polls `POST /v1/model-targets/check`. Startup, timeout, rate-limit, and temporary gateway failures are treated as loading states and rechecked automatically. Host, auth, missing-model, or missing-route failures stay blocked until the operator changes the configured gateway URL, API key, or model.

After the stack is running, use the end-to-end smoke gate:

```powershell
.\scripts\dev-check.ps1 -Smoke
```

This requires a reachable OpenAI-compatible model gateway. For an ingestion/search-only check, run:

```powershell
.\scripts\dev-check.ps1 -Smoke -SkipAsk
```

Before a release candidate or serious demo, run the full release gate:

```powershell
.\nexus.ps1 release-check
```

That gate runs tests, offline retrieval and answer evals, the production web build, smoke, backup/restore, and then prints the remaining manual UI checklist.

For search/retrieval changes, run the offline eval gate:

```powershell
.\nexus.ps1 eval-retrieval
```

It uses checked-in fixtures and does not require Docker or a model gateway.

For prompt, citation, refusal, or Ask-flow changes, run the offline answer eval:

```powershell
.\nexus.ps1 eval-answer
```

It uses checked-in fixtures and a deterministic model stub, so it does not require Docker or a model gateway.
