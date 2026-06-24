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

