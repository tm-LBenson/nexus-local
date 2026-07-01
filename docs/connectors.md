# Connector Handoff

Connector sources let an operator record customer-owned systems that should feed the knowledge base later, without storing secrets or pretending a direct connector worker exists today.

## Current Behavior

- Connector sources are created from Library with provider, resource ID, credential reference, and notes.
- Credential references are labels or secret-manager paths only. Nexus Local does not store connector secrets in the source record.
- Connector sources appear in the Library list and detail panel as handoff records.
- Filesystem actions are disabled for connector sources: path checks, import plans, scans, failure retries, and reindexing.
- Connector source changes are tenant-scoped and audited with provider/resource/credential reference metadata.

## Operator Pattern

Use connector sources when a customer needs SharePoint, OneDrive, ticket-system, CRM, case-management, or other SaaS data connected later:

1. Create a connector source with a clear name, such as `Support SharePoint` or `Case System Export`.
2. Set provider to a stable identifier, such as `sharepoint`, `onedrive`, `zendesk`, or `servicenow`.
3. Set resource ID to the customer-owned site, drive, project, queue, tenant, or export scope.
4. Set credential reference to the external secret path, vault key, or deployment note that the operator will resolve outside the app.
5. Use notes for customer-specific auth requirements, allow-listed networks, data boundaries, or sync caveats.

## Security Boundary

Connector handoff records are metadata. They should not contain API keys, passwords, refresh tokens, private keys, or full OAuth secrets.

When direct connector workers are added, the worker should resolve credentials from the deployment environment or customer secret store, apply least-privilege scopes, and write normal document-ingestion jobs through the same source/document pipeline used by mounted folders.

## Future Worker Contract

A direct connector worker should:

- Claim enabled connector sources by provider.
- Resolve credentials from the configured credential reference.
- Enumerate remote items with stable external IDs and update cursors.
- Apply include/exclude and policy profiles where they make sense.
- Create or update documents with source metadata and audit events.
- Preserve delete/reindex behavior through the same source ownership model as folder imports.
