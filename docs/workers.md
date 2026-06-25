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
6. Extract text from supported text, PDF, and OpenXML Office files.
7. Chunk extracted text.
8. Embed chunks through the configured embedder.
9. Delete old vectors for the document.
10. Upsert fresh vectors into the configured vector index.
11. Transition document `processing -> ready`.
12. Transition job `running -> succeeded`.

If ingestion fails, the worker transitions the document to `failed`, transitions the job to `failed`, and stores a concise `error_message` on the job. The document detail and activity views surface that message next to the retry action.

Current extraction support:

- Text-like UTF-8 files: `.txt`, `.md`, `.json`, `.html`, `.htm`, `.csv`, `.tsv`, and `.vtt`.
- PDF files: `.pdf`.
- OpenXML Office files: `.docx`, `.pptx`, and `.xlsx`.

Legacy binary Office formats such as `.doc`, `.ppt`, and `.xls` are intentionally not supported yet. They should be converted before upload or handled later by a dedicated converter service.

The API validates the supported filename/content type before storing uploads, so unsupported files fail fast with `415 Unsupported Media Type` instead of becoming queued jobs that later fail in the worker.

The CPU Compose profile runs the worker beside the API. It shares the same Postgres database and claims jobs through the repository contract.

The API search endpoint reads from the configured vector index. In containerized deployments, API and worker should share Qdrant or another external vector store; otherwise uploaded documents can be registered but their chunks will not be visible to API-side search.
