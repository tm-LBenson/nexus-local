# Roadmap

This roadmap turns the product vision into buildable phases. The intent is to keep every feature tied to a customer outcome, a trust requirement, and a release gate.

Nexus Local should grow in layers:

1. Make the private knowledge MVP dependable.
2. Make data import and retrieval useful at real customer scale.
3. Add governance before high-risk workflows.
4. Add controlled agents only after identity, audit, policy, and quality gates are credible.

## Roadmap Levels

- Broad roadmap: this file. It describes the sequence and why each phase exists.
- Phase roadmaps: one file per phase under [Roadmap Details](roadmap/README.md).
- Implementation queue: the next buildable slices inside each phase.
- Release gates: smoke tests, quality checks, security checks, and deployment checks that decide whether a slice is done.

## Phases

| Phase | Name | Goal | Exit gate |
| --- | --- | --- | --- |
| 1 | MVP stabilization | Make the current app reliable enough for repeat local demos and early internal use. | A fresh install can ingest, search, ask, audit, backup, restore, and recover from common failures. |
| 2 | Data sources and ingestion | Move from one-off uploads to managed knowledge sources. | A customer can import a folder/synced drive, see scan status, retry failures, and keep the knowledge base current. |
| 3 | Retrieval and answer quality | Make answers fast, cited, and trustworthy enough for real work. | Evaluation tests track recall, citation quality, latency, and failure behavior across fixture datasets. |
| 4 | Security, identity, and governance | Prepare for sensitive customer data and PII-heavy workflows. | Admins can configure auth, roles, audit review, retention, redaction, and policy controls. |
| 5 | Operator deployment experience | Make installs, upgrades, backups, diagnostics, and split deployments feel guided. | A non-developer operator can install, validate, upgrade, and troubleshoot common deployment shapes. |
| 6 | Workflow and agent foundation | Add tool-using workflows without losing auditability or human control. | Agents can run approved read-only workflows with policy checks, citations, and human escalation. |
| 7 | Production scale and multi-customer packaging | Make the platform repeatable across many customer deployments. | Release artifacts, support bundles, observability, upgrade paths, and environment profiles are predictable. |

## Phase Detail Index

- [Phase 1: MVP Stabilization](roadmap/phase-01-mvp-stabilization.md)
- [Phase 2: Data Sources and Ingestion](roadmap/phase-02-data-sources-ingestion.md)
- [Phase 3: Retrieval and Answer Quality](roadmap/phase-03-retrieval-answer-quality.md)
- [Phase 4: Security, Identity, and Governance](roadmap/phase-04-security-governance.md)
- [Phase 5: Operator Deployment Experience](roadmap/phase-05-operator-deployment.md)
- [Phase 6: Workflow and Agent Foundation](roadmap/phase-06-workflows-agents.md)
- [Phase 7: Production Scale and Packaging](roadmap/phase-07-production-scale.md)

## Current Build Priority

The next implementation work should stay inside Phase 1 and Phase 2 until the product is dependable as a private knowledge base.

Recommended order:

1. Improve streaming/loading UX for slow local models.
2. Add managed data-source records for folder imports.
3. Add a bulk import UI that creates source jobs instead of relying only on scripts.
4. Add ingestion progress, retry, and failure actions that are visible from the dashboard and library.
5. Add basic audit filters in Settings.
6. Add quality fixtures and evaluation scripts for retrieval/answer behavior.

## Release Gates

Every feature should have an explicit gate before it counts as done:

- Product gate: a real user can understand what happened and what to do next.
- Test gate: unit or integration tests cover the changed contract.
- Smoke gate: the end-to-end smoke path still passes.
- Deployment gate: Docker profiles still start cleanly.
- Trust gate: sensitive actions are authorized, auditable, and tenant-scoped.
- Documentation gate: setup or operator behavior is documented when it changes.

## Decision Rules

- Prefer infrastructure-portable designs over vendor-specific shortcuts.
- Prefer tenant-scoped product contracts before adding UI convenience.
- Prefer observable jobs over hidden background work.
- Prefer source-grounded answers over clever unsourced answers.
- Prefer policy and audit primitives before customer-facing agents.
- Prefer one dependable path over many partially working paths.

