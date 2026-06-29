# Backup and Restore

Nexus Local stores durable application data in three places for the Compose profiles:

- Postgres: workspaces, memberships, documents, chunks, jobs, conversations, and messages.
- MinIO: uploaded document objects.
- Qdrant: vector indexes.

NATS and Valkey are not included in the basic backup because they are queue/cache infrastructure. Model caches are also excluded because they can be downloaded again. For `prod-auth`, Caddy certificate/config volumes are not included in this first routine; back them up separately if you rely on Caddy-managed certificates.

## Create a Backup

Start the selected Compose profile first, then run:

```powershell
.\scripts\backup.ps1 -Profile cpu-lite
```

For the NAS plus desktop GPU profile:

```powershell
.\scripts\backup.ps1 -Profile split-nas-gpu
```

For the production auth profile:

```powershell
.\scripts\backup.ps1 -Profile prod-auth
```

The script writes a timestamped folder under `backups/` containing:

- `postgres.dump`
- `minio-data.tgz`
- `qdrant-storage.tgz`
- `manifest.json`
- `env.snapshot` when a root `.env` file exists

`env.snapshot` contains secrets and provider settings. Keep backup folders out of Git and copy them only to storage you trust.

By default, the script briefly stops API, worker, MinIO, and Qdrant before copying volume data, then starts them again. Postgres remains running for a logical `pg_dump`.

Use `-SkipServiceStop` only when you understand the consistency tradeoff:

```powershell
.\scripts\backup.ps1 -Profile cpu-lite -SkipServiceStop
```

## Validate Backup and Restore Preflight

Run a non-destructive backup smoke check against a running local stack:

```powershell
.\scripts\backup-smoke.ps1 -Profile cpu-lite
```

The same check is available through the launcher:

```powershell
.\nexus.ps1 backup-smoke
```

The smoke check creates a temporary backup, verifies the manifest, checks that the Postgres dump and MinIO/Qdrant archives can be read, runs restore preflight validation, and removes the temporary backup unless `-KeepBackup` is passed.

To validate an existing backup without restoring it:

```powershell
.\scripts\restore.ps1 `
  -BackupPath .\backups\nexus-local-cpu-lite-20260101-120000 `
  -ValidateOnly
```

## Restore a Backup

Start the same Compose profile with a compatible `.env`, then run:

```powershell
.\scripts\restore.ps1 -BackupPath .\backups\nexus-local-cpu-lite-20260101-120000
```

The restore script reads `manifest.json` to infer the profile when possible. You can override it:

```powershell
.\scripts\restore.ps1 `
  -BackupPath .\backups\nexus-local-prod-auth-20260101-120000 `
  -Profile prod-auth
```

Restore replaces Postgres data, MinIO objects, and Qdrant vectors in the selected Compose stack. Without `-Force`, the script asks you to type `RESTORE`.

For non-interactive restore:

```powershell
.\scripts\restore.ps1 -BackupPath .\backups\nexus-local-cpu-lite-20260101-120000 -Force
```

The script does not overwrite `.env`. If `env.snapshot` is present, review it manually before changing local credentials or provider settings.

## Recommended Habit

- Run a backup before destructive maintenance or upgrades.
- Keep at least one backup off the NAS or host running the stack.
- Run `.\nexus.ps1 backup-smoke` after backup/restore script changes or before a demo build.
- Test restore on a disposable profile before trusting a backup routine.
- Treat desktop GPU or rented GPU nodes as replaceable compute; back up the NAS or server that owns Postgres, MinIO, and Qdrant.
