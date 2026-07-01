# Testing Strategy

## Short Answer

Use TDD for the contracts that keep the system portable. Do not use strict TDD for every screen or every temporary implementation detail.

## Where Tests Come First

- Model routing: local GPU, AWS GPU, hosted provider, and mock targets must be interchangeable.
- Storage provider: MinIO, S3, and local development storage should share one contract.
- Vector search: pgvector and Qdrant migration should not change the app layer.
- Jobs: ingestion and AI turns need explicit state transitions.
- Auth and tenancy: access rules must be boring and heavily tested.
- API schemas: frontend/backend compatibility should be locked with fixtures.
- Health/readiness: deployment profiles should fail clearly when a dependency is missing.

## Where TDD Is Not Worth It Yet

- Early frontend layout exploration.
- One-off admin screens before the domain settles.
- Internal helper code that has no user-visible contract.
- Throwaway migration scripts.

## Practical Rule

If changing it later would break portability, security, tenancy, data integrity, or deployments, test it early. If it is just a first-draft UI expression, move fast and add tests when the workflow stabilizes.

## End-to-End Smoke Gate

Use the smoke script against a running stack before calling a build demo-ready:

```powershell
.\scripts\dev-smoke.ps1
```

The script creates a workspace, uploads `fixtures/smoke/nexus-smoke.md`, waits for ingestion, searches for a unique run phrase, asks over the uploaded document, and verifies that conversation history contains the turn. It deletes the temporary document and temporary workspace; pass `-TenantId` to reuse an existing workspace. Use `-SkipAsk` only when the model gateway is intentionally unavailable and you want to verify ingestion and search by themselves.

## Release Candidate Gate

Use the release check before tagging, demoing, or handing a build to another machine:

```powershell
.\nexus.ps1 release-check
```

This command runs API and launcher tests, the retrieval eval gate, the web production build, the end-to-end smoke gate, and the backup/restore round trip. It finishes by printing the manual UI checklist that still needs human eyes: setup blocking, first-run sample, dashboard, library, activity, settings, and slow-model answer states.

## Retrieval Eval Gate

Use the offline retrieval eval when touching search, chunking, source metadata, filters, embeddings, or answer grounding:

```powershell
.\nexus.ps1 eval-retrieval
```

The fixture at `fixtures/eval/retrieval-baseline.json` builds an in-memory index with hash embeddings, runs the normal search service, and verifies that support/governance questions retrieve the expected document chunks within the expected rank. The report includes aggregate recall, mean expected rank, p95 case latency, and no-hit case pass counts. It does not need Docker, a model gateway, or a GPU.

## Backup/Restore Gate

Use the fast backup validation after touching backup packaging or before a casual local demo:

```powershell
.\scripts\dev-check.ps1 -BackupSmoke
```

Use the full local restore round trip before a release candidate or after changing restore behavior:

```powershell
.\scripts\dev-check.ps1 -BackupSmoke -BackupRestoreRoundTrip
```

The round trip creates a temporary workspace/document, backs it up, deletes it, restores the backup, verifies the restored document metadata, object download, and search index, then deletes the temporary workspace again.
