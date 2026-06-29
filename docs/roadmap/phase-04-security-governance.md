# Phase 4: Security, Identity, and Governance

## Outcome

Nexus Local can handle sensitive customer data with credible identity, permissions, audit review, retention controls, redaction, and policy gates.

## Why This Matters

The product vision includes PII-heavy agency and business workflows. That cannot be bolted on after agents are already answering account-specific questions. Governance needs to become a product layer before high-risk automation.

## Scope

- Production auth hardening.
- Role and permission expansion.
- Audit search, export, and retention.
- PII detection and redaction primitives.
- Policy rules for answer scope.
- Human review and escalation hooks.
- Tenant-level data retention controls.

## Implementation Slices

| Slice | Build | Gate |
| --- | --- | --- |
| 4.1 | Audit filters, exports, and retention settings. | Admin can produce an audit packet for a tenant/date range. |
| 4.2 | Permission expansion for sources, audits, settings, policies, and agent runs. | Non-admin users cannot access admin-only surfaces. |
| 4.3 | Identity provider deployment examples beyond dev/trusted header. | Production auth path is reproducible. |
| 4.4 | PII detection pipeline for ingested text and answers. | Fixture PII is detected and labeled. |
| 4.5 | Redaction/minimization rules for source snippets and answers. | Restricted answers hide configured sensitive fields. |
| 4.6 | Policy decision service for answer/tool allow, deny, review. | Policy decisions are auditable and testable. |

## Dependencies

- Tenant isolation must remain strict.
- Audit events must exist for sensitive actions.
- Phase 2 source management should identify where data came from.
- Phase 3 eval framework should test refusal and redaction behavior.

## Release Gate

- Authorization tests for every privileged endpoint.
- PII fixture tests.
- Audit export test.
- Manual production-auth deployment check.

## First Build Candidate

Start with Slice 4.1 after Phase 1 audit UI exists. Audit search/export is useful now and prepares for regulated customer conversations.

