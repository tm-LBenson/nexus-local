# Phase 5: Operator Deployment Experience

## Outcome

An operator can install, configure, validate, upgrade, back up, restore, and troubleshoot Nexus Local across Windows Docker Desktop, Linux, NAS plus GPU, local GPU, and cloud deployments.

## Why This Matters

The product may be deployed by you, a customer IT admin, or a technical power user. A powerful app that is hard to install will fail before users ever reach the knowledge workflow.

## Scope

- Guided launcher improvements.
- TUI/GUI setup flow.
- Preflight checks and recommended profiles.
- Model gateway validation and troubleshooting.
- Backup/restore workflows.
- Upgrade/migration flow.
- Support bundle generation.
- Split deploy documentation and validation.

## Implementation Slices

| Slice | Build | Gate |
| --- | --- | --- |
| 5.1 | Launcher health dashboard with actions for start, stop, logs, reset, backup, restore. | `.\nexus.ps1` can manage the stack without flags. |
| 5.2 | Gateway setup assistant for local GPU, external GPU, hosted-compatible endpoints. | User can configure a working model target from prompts. |
| 5.3 | Cross-platform launcher parity for PowerShell and POSIX shell. | Windows and Nobara/Linux test runs match behavior. |
| 5.4 | Backup/restore command path with dry-run and validation. | Restore can be verified against a test stack. |
| 5.5 | Upgrade path with migration preview and rollback guidance. | Version upgrade does not surprise operators. |
| 5.6 | Support bundle with sanitized config, service status, logs, and smoke output. | Operator can send useful diagnostics without leaking secrets. |

## Dependencies

- Existing Compose profiles.
- Existing setup scripts.
- Phase 1 smoke and backup checks.

## Release Gate

- Windows Docker Desktop launcher check.
- Linux launcher check.
- Split app/GPU config check.
- Smoke test after backup/restore and after upgrade.

## First Build Candidate

Start with Slice 5.1 when the app core stabilizes. It directly addresses the desired "one command, pick options, run everything" flow.

