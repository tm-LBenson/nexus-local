# Workers

Workers claim durable jobs and advance product state outside the request path.

## Source Scans and Document Ingestion

Location: `services/api/internal/worker`

Runnable command: `services/api/cmd/worker`

Container entrypoint: `/worker`

The worker executable processes two durable job types from the same database-backed queue:

- `source_scan` jobs for managed folder/synced-drive/network-share sources.
- `document_ingestion` jobs for parsing and indexing uploaded or scanned documents.

Source scheduling behavior:

1. On each worker poll, find sources whose `next_scan_at` is due.
2. Skip archived/scanning sources and sources with an already active `source_scan` job.
3. Queue a normal `source_scan` job for each due source.
4. Advance `next_scan_at` by `scan_interval_minutes` so duplicate scans are not queued every poll.
5. Manual scans and completed/failed scheduled scans also advance the next due scan.

Source scan behavior:

1. Claim the next queued `source_scan` job.
2. Load the data source by tenant and resource ID.
3. Transition source `active/failed -> scanning`.
4. Walk the source root path from the worker machine/container.
5. Apply source include/exclude patterns, then skip symlinks, hidden names, common cache/build folders, oversized files, and unsupported document types.
6. Hash each supported file with SHA-256.
7. Skip unchanged files when the latest imported entry for the same path has the same content hash.
8. Upload new or changed supported files through the normal document upload path.
9. For changed source paths, delete the prior document after the replacement upload succeeds.
10. After a clean walk, delete prior documents for source paths that no longer exist.
11. Queue one `document_ingestion` job per imported file.
12. Persist per-file scan entries with imported/skipped/failed/deleted outcome, reason, message, size, content hash, and imported document ID when available.
13. Transition source `scanning -> active`, stamp `last_scan_at`, and persist imported/skipped/failed counts.
14. Transition the source scan job `running -> succeeded`.

The source path must be visible to the worker process. In Docker deployments, mount the folder, synced drive, or network share into the worker container and use the container-visible path in the source record. For example, a Windows folder can be mounted as `/sources/customer-docs`, and the source root should use `/sources/customer-docs`, not the Windows host path.

Default source scan safety:

- Maximum file size: 10 MiB.
- Source include/exclude patterns use source-relative paths such as `**/*.md`, `cases/**`, `archive/**`, or `*.draft.md`; excludes win over includes.
- Files outside include patterns are skipped with reason `not_included`.
- Files or directories matching exclude patterns are skipped with reason `excluded`.
- Hidden names are skipped.
- Common cache/build folders are skipped, including `.git`, `node_modules`, `.cache`, `__pycache__`, `.next`, `dist`, `build`, `target`, `tmp`, and virtual environment folders.
- Unsupported document types are counted as skipped, not failed.
- Unchanged supported files are counted as skipped with reason `unchanged`.
- Missing files that were previously imported are counted as skipped with outcome `deleted` and reason `missing`.

If a source scan cannot access the root folder or hits file-level read/upload failures, the worker transitions the source to `failed`, transitions the job to `failed`, stores imported/skipped/failed counts on the source, records file-level failure entries, and stores a concise count summary plus the first failure in `error_message`. Source detail exposes the newest scan summary plus paged, filterable, exportable file outcomes for import review.

Changed files replace the previously imported document for the same source path. Missing files and files removed by the current source filters delete the previously imported document for that source path after the current scan completes without file-level failures.

Document ingestion behavior:

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

Document ingestion can be parallelized with `WORKER_DOCUMENT_CONCURRENCY`. The default is `1`, which is safest for small local machines and weak model/embedding gateways. Increase it only when Postgres, object storage, embeddings, and Qdrant have enough capacity. Source scans remain single-threaded; they queue document ingestion jobs that the worker can then process concurrently.

Current extraction support:

- Text-like UTF-8 files: `.txt`, `.md`, `.json`, `.html`, `.htm`, `.csv`, `.tsv`, and `.vtt`.
- PDF files: `.pdf`.
- OpenXML Office files: `.docx`, `.pptx`, and `.xlsx`.

Legacy binary Office formats such as `.doc`, `.ppt`, and `.xls` are intentionally not supported yet. They should be converted before upload or handled later by a dedicated converter service.

The API validates the supported filename/content type before storing uploads, so unsupported files fail fast with `415 Unsupported Media Type` instead of becoming queued jobs that later fail in the worker.

The CPU Compose profile runs the worker beside the API. It shares the same Postgres database and claims jobs through the repository contract.

The API search endpoint reads from the configured vector index. In containerized deployments, API and worker should share Qdrant or another external vector store; otherwise uploaded documents can be registered but their chunks will not be visible to API-side search.
