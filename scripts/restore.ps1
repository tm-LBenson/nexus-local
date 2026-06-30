param(
  [Parameter(Mandatory = $true)]
  [string]$BackupPath,
  [string]$Profile = "",
  [string]$HelperImage = "alpine:3.20",
  [switch]$Force,
  [switch]$NoRestart,
  [switch]$ValidateOnly
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $root ".env"
$composeHelper = Join-Path (Join-Path $PSScriptRoot "lib") "compose.ps1"
. $composeHelper

function Require-Command($name) {
  if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
    throw "$name is required but was not found on PATH"
  }
}

function Invoke-Docker($arguments) {
  & docker @arguments
  if ($LASTEXITCODE -ne 0) {
    throw "docker $($arguments -join ' ') failed"
  }
}

function Invoke-DockerOutput($arguments) {
  $output = & docker @arguments
  if ($LASTEXITCODE -ne 0) {
    throw "docker $($arguments -join ' ') failed"
  }
  return $output
}

function Require-BackupFile($path) {
  if (-not (Test-Path $path)) {
    throw "Required backup file is missing: $path"
  }
  $item = Get-Item -LiteralPath $path
  if ($item.Length -le 0) {
    throw "Required backup file is empty: $path"
  }
  return $item
}

function Get-ServiceContainer($composeArgs, $service) {
  $rawID = Invoke-DockerOutput ($composeArgs + @("ps", "-q", $service)) | Select-Object -First 1
  $id = ""
  if ($rawID) {
    $id = $rawID.Trim()
  }
  if (-not $id) {
    throw "Container for service '$service' was not found. Start the selected Compose profile before restoring."
  }
  return $id
}

function Restore-VolumeFromArchive($containerID, $targetPath, $archiveName, $backupFullPath) {
  Write-Host "Restoring $archiveName..."
  Invoke-Docker @(
    "run", "--rm",
    "--volumes-from", $containerID,
    "-v", "$backupFullPath`:/backup:ro",
    $HelperImage,
    "sh", "-lc",
    "mkdir -p $targetPath && find $targetPath -mindepth 1 -maxdepth 1 -exec rm -rf {} + && tar -xzf /backup/$archiveName -C $targetPath"
  )
}

function Start-RestoredServices($composeArgs) {
  Invoke-Docker ($composeArgs + @("start", "minio", "qdrant"))
  Start-Sleep -Seconds 5
  Invoke-Docker ($composeArgs + @("start", "api", "worker"))
}

Require-Command docker

$backupFullPath = (Resolve-Path -LiteralPath $BackupPath).Path
$manifestPath = Join-Path $backupFullPath "manifest.json"
$postgresDump = Join-Path $backupFullPath "postgres.dump"
$minioArchive = Join-Path $backupFullPath "minio-data.tgz"
$qdrantArchive = Join-Path $backupFullPath "qdrant-storage.tgz"

Require-BackupFile $postgresDump | Out-Null
Require-BackupFile $minioArchive | Out-Null
Require-BackupFile $qdrantArchive | Out-Null

$manifest = $null
if (Test-Path $manifestPath) {
  $manifest = Get-Content -Raw -Path $manifestPath | ConvertFrom-Json
  if ([int]$manifest.version -ne 1) {
    throw "Unexpected backup manifest version: $($manifest.version)"
  }
  if ($manifest.components.postgres -ne "postgres.dump" -or
      $manifest.components.minio -ne "minio-data.tgz" -or
      $manifest.components.qdrant -ne "qdrant-storage.tgz") {
    throw "Backup manifest component names are not recognized."
  }
}
if (-not $Profile) {
  if ($manifest -and $manifest.profile) {
    $Profile = [string]$manifest.profile
  } else {
    $Profile = Resolve-NexusDeploymentProfile -Profile "" -EnvFile $envFile
  }
}
$selectedProfile = Resolve-NexusDeploymentProfile -Profile $Profile -EnvFile $envFile
if ($manifest -and $manifest.profile -and [string]$manifest.profile -ne $selectedProfile) {
  throw "Backup profile '$($manifest.profile)' does not match selected profile '$selectedProfile'."
}

$composeConfig = Get-NexusComposeConfig -Root $root -EnvFile $envFile -Profile $selectedProfile
$composeArgs = $composeConfig.Args
$postgresContainer = Get-ServiceContainer $composeArgs "postgres"
$minioContainer = Get-ServiceContainer $composeArgs "minio"
$qdrantContainer = Get-ServiceContainer $composeArgs "qdrant"

if ($ValidateOnly) {
  Write-Host "Restore validation passed."
  Write-Host "Profile: $selectedProfile"
  Write-Host "Backup path: $backupFullPath"
  Write-Host "Postgres container: $postgresContainer"
  Write-Host "MinIO container: $minioContainer"
  Write-Host "Qdrant container: $qdrantContainer"
  exit 0
}

if (-not $Force) {
  Write-Host "Restore target profile: $selectedProfile"
  Write-Host "Backup path: $backupFullPath"
  Write-Host "This replaces Postgres data, MinIO objects, and Qdrant vectors for the selected Compose stack."
  $answer = Read-Host "Type RESTORE to continue"
  if ($answer -ne "RESTORE") {
    Write-Host "Restore canceled."
    exit 0
  }
}

$stoppedServices = @("api", "worker", "minio", "qdrant")

try {
  Write-Host "Stopping app and cold-copy services..."
  Invoke-Docker ($composeArgs + @("stop") + $stoppedServices)

  Write-Host "Restoring postgres.dump..."
  Invoke-Docker ($composeArgs + @(
      "cp", $postgresDump,
      "postgres:/tmp/nexus-local-restore-postgres.dump"
    ))
  Invoke-Docker ($composeArgs + @(
      "exec", "-T", "postgres", "sh", "-lc",
      'pg_restore --clean --if-exists --no-owner --no-acl -U "$POSTGRES_USER" -d "$POSTGRES_DB" /tmp/nexus-local-restore-postgres.dump'
    ))
  Invoke-Docker ($composeArgs + @(
      "exec", "-T", "postgres", "rm", "-f", "/tmp/nexus-local-restore-postgres.dump"
    ))

  Restore-VolumeFromArchive $minioContainer "/data" "minio-data.tgz" $backupFullPath
  Restore-VolumeFromArchive $qdrantContainer "/qdrant/storage" "qdrant-storage.tgz" $backupFullPath

  Write-Host ""
  Write-Host "Restore complete: $backupFullPath"
  $envSnapshot = Join-Path $backupFullPath "env.snapshot"
  if (Test-Path $envSnapshot) {
    Write-Host "Review env.snapshot manually if credentials or provider settings also need to be restored."
  }
} finally {
  if (-not $NoRestart) {
    Write-Host "Starting restored services..."
    Start-RestoredServices $composeArgs
  }
}
