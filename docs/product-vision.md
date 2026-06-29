# Product Vision

Nexus Local is a private AI operations layer for organizations that cannot casually send their data to a hosted assistant. It should be deployable on customer-owned hardware, a secure cloud account, rented GPU infrastructure, or a split setup where the application data stays on one machine and inference runs on another.

The product starts as a knowledge workspace: customers import documents, synced folders, exports, support artifacts, case notes, policies, runbooks, and structured records. Nexus Local parses and indexes that material into tenant-scoped knowledge bases, then lets authorized users search and ask questions with citations.

The longer-term direction is a controlled agent platform. Agents should be able to use the same knowledge base, call approved tools, check identity, enforce policy, produce auditable actions, and hand off to a human when risk is too high.

## Target Customers

- Agencies and businesses with PII, regulated records, private operations data, or internal technical knowledge.
- Support and engineering teams that want a local assistant over tickets, HAR files, logs, product docs, and previous resolutions.
- Government or service teams that need status answers from private records after identity checks.
- Small teams that want CPU-only or remote-AI deployments before buying dedicated GPU hardware.

## Product Modes

### Knowledge Mode

Users bring their own files or synced directories. The app imports supported content, tracks ingestion state, indexes chunks, and lets users ask questions with visible sources. This mode should not be tied to one source such as Obsidian; it should handle local folders, OneDrive/SharePoint/Teams synced files, exports, network shares, object storage, and future connectors.

### Case Analysis Mode

Users upload evidence such as HAR files, logs, tickets, configuration snippets, screenshots, or customer notes. The assistant compares the issue against internal docs and previous cases, explains likely causes, and cites the records it used.

### Service Agent Mode

An authenticated user asks for a status update or account-specific answer. The agent verifies identity, checks authorization, retrieves the minimum necessary records, redacts sensitive details, answers within policy, and logs the interaction for review.

### Operator Mode

Admins manage deployments, model gateways, ingestion health, data sources, backups, members, audit logs, and policy. This should stay out of the daily user path unless something needs attention.

## Trust Requirements

- Tenant isolation: every document, record, vector, conversation, job, and audit event belongs to a tenant.
- Identity-aware access: production deployments must integrate with a real identity provider or trusted auth gateway.
- Permission checks: users and agents need explicit permission for reading documents, using AI, managing users, importing sources, and viewing audits.
- Audit trail: sensitive actions must leave a queryable event trail with actor, tenant, action, resource, outcome, metadata, and timestamp.
- Source-grounded answers: the assistant should show what records it used and make uncertainty visible.
- PII controls: future policy layers should support redaction, minimization, refusal, approval gates, retention rules, and purpose-specific access.
- Human handoff: the system should know when not to answer, especially for regulated, account-specific, or destructive workflows.

## Deployment Principles

- The app/data layer should be portable across Docker Desktop, Linux servers, NAS devices, on-prem machines, and cloud VMs.
- Inference should sit behind an OpenAI-compatible gateway so local GPUs, laptop GPUs, rented GPUs, or hosted providers can be swapped without changing product code.
- Storage should prefer Postgres, S3-compatible object storage, and a replaceable vector index instead of vendor-specific services.
- A guided launcher should own first-run setup, health checks, model gateway configuration, and clear next steps.
- The same architecture should scale from one-person local testing to many customer deployments that provide their own hardware.

## Near-Term Product Gaps

- Bulk import UI and connector jobs for arbitrary folders and synced document roots.
- Better job progress, loading states, cancellation, retry, and failure detail in the UI.
- Streaming answer UX with visible thinking/progress states and clear timeout recovery.
- Audit filters, export, and policy review surfaces.
- Data-source management: scan schedules, include/exclude rules, file type summaries, and stale-source detection.
- PII and policy primitives before any external self-service agent use case.
- Evaluation suites for answer quality, citation quality, latency, and refusal behavior.
