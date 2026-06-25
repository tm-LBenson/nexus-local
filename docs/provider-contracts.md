# Provider Contracts

Provider contracts are the main defense against cloud lock-in and hardware lock-in.

## Runtime Presets

`PROVIDER_PRESET` is a setup/runtime label for operators and the UI. It does not replace the concrete provider settings.

- `starter`: OpenAI-compatible chat with hash embeddings.
- `semantic`: OpenAI-compatible chat with OpenAI-compatible embeddings.

The concrete source of truth remains:

- `MODEL_GATEWAY_BASE_URL`
- `MODEL_GATEWAY_API_KEY`
- `GENERAL_MODEL_ID`
- `EMBEDDING_BACKEND`
- `EMBEDDING_BASE_URL`
- `EMBEDDING_API_KEY`
- `EMBEDDING_MODEL`
- `EMBEDDING_DIMENSIONS`

## Model Gateway

The API should call an OpenAI-compatible HTTP interface. The implementation behind that URL can be:

- vLLM on a local desktop GPU
- Ollama on a local machine
- LiteLLM routing across several providers
- AWS GPU instance running vLLM
- Hosted commercial model API

The app should store target names like `general`, `email-revision`, or `document-rag`, not provider-specific model details throughout product code.

The first concrete adapter lives at `services/api/internal/providers/openaicompat`. It posts to `/chat/completions`, supports optional bearer auth, and maps OpenAI-compatible responses into the backend `ModelGateway` contract.

Runtime model routing lives in `services/api/internal/runtime`. Product services call a target such as `general`; runtime resolves that target to an OpenAI-compatible gateway and model ID. The conversation ask flow stores user and assistant messages while keeping provider details outside the app layer.

## Object Storage

Use S3-compatible operations:

- put object
- get object
- delete object
- presigned download
- metadata lookup

MinIO is the local default. AWS S3 or another S3-compatible service should require configuration changes, not code changes.

Implemented object adapters:

- `providers/objectstore/memory`: local development and tests.
- `providers/objectstore/minio`: S3-compatible object storage for MinIO and compatible providers.

## Vector Search

The product layer should ask for semantic search by tenant, document, and query vector. It should not know whether Qdrant, pgvector, or another vector store is underneath.

Implemented vector adapters:

- `providers/vector/memory`: local development and tests.
- `providers/vector/qdrant`: Qdrant REST adapter using collection creation, point upsert, filter delete, and query-points search.

The Qdrant adapter follows the Qdrant REST API for creating collections, upserting points, deleting by filter, and querying points.

## Embeddings

Implemented embedding adapters:

- `providers/embeddings/hash`: deterministic local embeddings for development and tests.
- `providers/embeddings/openaicompat`: OpenAI-compatible `/embeddings` adapter for TEI, hosted APIs, or a self-hosted gateway that returns indexed float embeddings.

The hash embedder is not a semantic model. It lets the ingestion pipeline run anywhere while we wire the rest of the system. Use `EMBEDDING_BACKEND=openai-compatible`, `EMBEDDING_BASE_URL`, `EMBEDDING_API_KEY`, and `EMBEDDING_MODEL` for real retrieval quality.

For a self-hosted semantic baseline, use a TEI or OpenAI-compatible embedding service with `BAAI/bge-small-en-v1.5` and `EMBEDDING_DIMENSIONS=384`. Hosted embedding providers are fine too; set dimensions to the provider/model output size.

When the API and worker run as separate processes, use a shared vector backend such as Qdrant. The memory vector adapter is useful for unit tests and single-process experiments, but it is not shared across API and worker containers.

## Jobs

Long-running work should use explicit states:

- queued
- running
- retrying
- succeeded
- failed
- canceled

Progress events should be append-only and safe to stream, poll, or replay.
