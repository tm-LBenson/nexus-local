# Workers

Workers claim durable jobs and advance product state outside the request path.

## Document Ingestion

Location: `services/api/internal/worker`

Current behavior:

1. Claim the next queued job.
2. Require `document_ingestion` with `resource_type=document`.
3. Load the document by tenant and resource ID.
4. Transition document `uploaded -> processing -> ready`.
5. Transition job `running -> succeeded`.

This is intentionally a state-flow placeholder. The next version should replace the placeholder section with:

- object-store read
- text extraction
- chunking
- embedding
- vector upsert
- progress events

The state flow is tested now so the future extraction work can focus on content behavior without changing job semantics.

