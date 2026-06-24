# Workers

Workers claim durable jobs and advance product state outside the request path.

## Document Ingestion

Location: `services/api/internal/worker`

Runnable command: `services/api/cmd/worker`

Container entrypoint: `/worker`

Current behavior:

1. Claim the next queued job.
2. Require `document_ingestion` with `resource_type=document`.
3. Load the document by tenant and resource ID.
4. Transition document `uploaded -> processing`.
5. Read the source object from configured object storage.
6. Extract UTF-8 text from text-like files.
7. Chunk extracted text.
8. Embed chunks through the configured embedder.
9. Delete old vectors for the document.
10. Upsert fresh vectors into the configured vector index.
11. Transition document `processing -> ready`.
12. Transition job `running -> succeeded`.

Current extraction support is intentionally limited to text-like files: `.txt`, `.md`, `.json`, `.html`, `.htm`, `.csv`, `.tsv`, and `.vtt`. PDF/Office extraction should be added behind the same extractor boundary.

The CPU Compose profile runs the worker beside the API. It shares the same Postgres database and claims jobs through the repository contract.

The API search endpoint reads from the configured vector index. In containerized deployments, API and worker should share Qdrant or another external vector store; otherwise uploaded documents can be registered but their chunks will not be visible to API-side search.
