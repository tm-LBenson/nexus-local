# Provider Contracts

Provider contracts are the main defense against cloud lock-in and hardware lock-in.

## Model Gateway

The API should call an OpenAI-compatible HTTP interface. The implementation behind that URL can be:

- vLLM on a local desktop GPU
- Ollama on a local machine
- LiteLLM routing across several providers
- AWS GPU instance running vLLM
- Hosted commercial model API

The app should store target names like `general`, `email-revision`, or `document-rag`, not provider-specific model details throughout product code.

## Object Storage

Use S3-compatible operations:

- put object
- get object
- delete object
- presigned download
- metadata lookup

MinIO is the local default. AWS S3 or another S3-compatible service should require configuration changes, not code changes.

## Vector Search

The product layer should ask for semantic search by tenant, document, and query vector. It should not know whether Qdrant, pgvector, or another vector store is underneath.

## Jobs

Long-running work should use explicit states:

- queued
- running
- retrying
- succeeded
- failed
- canceled

Progress events should be append-only and safe to stream, poll, or replay.

