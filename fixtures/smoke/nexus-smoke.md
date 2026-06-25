# Nexus Local Smoke Fixture

Run ID: {{SMOKE_RUN_ID}}

This fixture verifies that Nexus Local can create a workspace, upload a document, ingest it with the worker, index it for retrieval, answer over retrieved context, and preserve conversation history.

Unique smoke phrase: {{SMOKE_NEEDLE}}

Expected release-gate behavior:

- The uploaded document reaches the ready state.
- Search returns this document and includes the unique smoke phrase.
- Ask retrieves this document as context.
- Conversation history contains the user prompt and assistant response.
