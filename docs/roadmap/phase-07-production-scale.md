# Phase 7: Production Scale and Packaging

## Outcome

Nexus Local can be deployed, supported, upgraded, and observed across many customer environments without each install becoming a custom project.

## Why This Matters

The long-term goal is not one local demo. It is a repeatable self-hosted product that can run where customers have the right hardware, data, and security posture.

## Scope

- Release packaging.
- Versioned migrations.
- Observability dashboards.
- Support bundle workflow.
- Multi-node direction.
- GPU scheduling direction.
- Customer environment profiles.
- Documentation for deployment classes and hardware sizing.

## Implementation Slices

| Slice | Build | Gate |
| --- | --- | --- |
| 7.1 | Versioned release artifacts and changelog discipline. | Operators know exactly what changed. |
| 7.2 | Upgrade and migration test matrix. | Supported profiles upgrade from prior release. |
| 7.3 | Observability profile with metrics, traces, logs, and dashboards. | Operator can diagnose latency, ingestion, and gateway failures. |
| 7.4 | Environment profile templates for small office, GPU workstation, NAS split, cloud GPU, and enterprise. | Profile docs map to config and hardware expectations. |
| 7.5 | Multi-node/k3s architecture spike. | Future scale path is validated before implementation. |
| 7.6 | Customer handoff package: install guide, support bundle, backup guide, security checklist. | New deployment is repeatable by checklist. |

## Dependencies

- Stable app contracts from earlier phases.
- Operator deployment workflows.
- Observability hooks from workers, API, model gateway, and UI.

## Release Gate

- Release checklist completed.
- Upgrade from previous release tested.
- Support bundle generated and inspected.
- Deployment docs verified against at least two environments.

## First Build Candidate

Start with Slice 7.1 once the MVP is stable enough to tag. Versioned releases should begin before external installs become common.

