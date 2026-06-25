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

The script creates a workspace, uploads `fixtures/smoke/nexus-smoke.md`, waits for ingestion, searches for a unique run phrase, asks over the uploaded document, and verifies that conversation history contains the turn. It deletes the temporary document, but the smoke workspace remains until workspace deletion exists; pass `-TenantId` to reuse an existing workspace. Use `-SkipAsk` only when the model gateway is intentionally unavailable and you want to verify ingestion and search by themselves.
