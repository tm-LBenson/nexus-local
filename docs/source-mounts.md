# Source Mounts

Managed sources are scanned by the worker container. The path entered in Dashboard Add data or Library must be visible inside that container.

## Quick Path

During guided setup, choose one readable source root from the machine running Docker. The launcher writes that host path to `NEXUS_SOURCE_HOST_PATH` and exposes it to the worker as `NEXUS_SOURCE_CONTAINER_PATH`, usually `/sources/primary`.

Then start with the printed Compose command or run:

```powershell
.\nexus.ps1 start
```

In Dashboard Add data, use **Connect + scan** with paths under `/sources/primary`, for example:

- `/sources/primary`
- `/sources/primary/SharePoint`
- `/sources/primary/exports`
- `/sources/primary/customers/acme`

Use the quick presets for common starting points. They prefill source type, path, include rules, exclude rules, and schedule for folders, vaults, team drives, exports, and runbooks. Library exposes the same managed sources with deeper controls for saved views, retry, reindex, CSV review, archive, and cleanup.

## Host Examples

| Source type | Host path example | Library path |
| --- | --- | --- |
| Windows OneDrive or Teams sync | `C:\Users\you\OneDrive - Company\Docs` | `/sources/primary` |
| Windows local folder | `D:\KnowledgeBase` | `/sources/primary` |
| Linux folder | `/srv/customer-docs` | `/sources/primary` |
| NAS SMB/NFS mounted on Linux | `/mnt/nas/customer-docs` | `/sources/primary` |
| Export drop folder | `/srv/imports/zendesk-export` | `/sources/primary` or `/sources/primary/zendesk-export` |

For multiple source records, mount a parent folder once and create source paths below it.

## Notes

- The mount is read-only by default.
- Do not use this mount for hot Postgres or Qdrant data.
- For Docker Desktop on Windows, make sure the drive or folder is shared with Docker Desktop.
- For NAS shares, mount the share on the Docker host first, then bind that mounted folder into Nexus Local.
- If a source scan fails with "root not accessible", check the host path, Docker file sharing, and the Library path.
