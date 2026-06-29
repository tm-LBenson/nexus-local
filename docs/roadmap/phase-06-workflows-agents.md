# Phase 6: Workflow and Agent Foundation

## Outcome

Nexus Local can run controlled workflows and read-only agents over customer knowledge while enforcing identity, policy, audit, and human escalation.

## Why This Comes Later

Agents are powerful only when the knowledge base, auth, audit, policy, and evaluation layers are strong. Without those layers, agents increase risk faster than they increase value.

## Scope

- Agent run domain model.
- Tool registry and approvals.
- Read-only workflow engine.
- Policy checks before retrieval, answer, and tool use.
- Human approval/escalation queue.
- Agent evaluation fixtures.
- Service-status workflow examples.

## Implementation Slices

| Slice | Build | Gate |
| --- | --- | --- |
| 6.1 | Agent run records with state, actor, tenant, objective, policy outcome, and audit events. | Every run is traceable. |
| 6.2 | Tool registry for approved read-only tools. | Agents cannot call unregistered tools. |
| 6.3 | Workflow runner for deterministic multi-step read-only flows. | Fixture workflow runs without model creativity. |
| 6.4 | Human review queue for policy-blocked or high-risk answers. | User can escalate instead of forcing an answer. |
| 6.5 | Service-status example workflow with identity confirmation placeholders. | Demo shows account/status style use case without exposing real PII. |
| 6.6 | Agent eval suite for refusal, source use, and policy compliance. | Agent regressions are measurable. |

## Dependencies

- Phase 3 evaluation system.
- Phase 4 policy and PII primitives.
- Audit and permission model.
- Reliable job/workflow infrastructure.

## Release Gate

- Agent/tool authorization tests.
- Policy decision tests.
- Audit trail for every agent step.
- Human escalation browser check.
- Evaluation fixture suite.

## First Build Candidate

Start with Slice 6.1 only after governance primitives exist. Agent run records are the foundation for every later workflow.

