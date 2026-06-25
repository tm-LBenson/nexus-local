param(
  [Parameter(Mandatory = $true)]
  [string]$BackupPath,
  [string]$Profile = "",
  [string]$HelperImage = "alpine:3.20",
  [switch]$Force,
  [switch]$NoRestart
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $root ".env"
$profileNames = @("cpu-lite", "split-nas-gpu", "gpu-local", "prod-auth")

function Require-Command($name) {
  if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
    throw "$name is required but was not found on PATH"
  }
}

function Resolve-Profile($value) {
  $normalized = $value.Trim().ToLowerInvariant()
  if ($profileNames -notcontains $normalized) {
    throw "Unknown profile '$value'. Use one of: $($profileNames -join ', ')"
  }
  return $normalized
}

function Get-ComposeArgs($selectedProfile) {
  $args = @("compose")
  if (Test-Path $envFile) {
    $args += @("--env-file", $envFile)
  }

  switch ($selectedProfile) {
    "cpu-lite" {
      $args += @("-f", (Join-Path $root "deploy\compose\compose.cpu.yml"))
    }
    "split-nas-gpu" {
      $args += @(
        "-f", (Join-Path $root "deploy\compose\compose.cpu.yml"),
        "-f", (Join-Path $root "deploy\compose\compose.split-nas-gpu.yml")
      )
    }
    "gpu-local" {
      $args += @(
        "-f", (Join-Path $root "deploy\compose\compose.cpu.yml"),
        "-f", (Join-Path $root "deploy\compose\compose.gpu.yml"),
        "--profile", "gpu"
      )
    }
    "prod-auth" {
      $args += @("-f", (Join-Path $root "deploy\compose\compose.prod-auth.yml"))
    }
  }
  return $args
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

Require-Command docker

$backupFullPath = (Resolve-Path -LiteralPath $BackupPath).Path
$manifestPath = Join-Path $backupFullPath "manifest.json"
$postgresDump = Join-Path $backupFullPath "postgres.dump"
$minioArchive = Join-Path $backupFullPath "minio-data.tgz"
$qdrantArchive = Join-Path $backupFullPath "qdrant-storage.tgz"

foreach ($requiredFile in @($postgresDump, $minioArchive, $qdrantArchive)) {
  if (-not (Test-Path $requiredFile)) {
    throw "Required backup file is missing: $requiredFile"
  }
}

$manifest = $null
if (Test-Path $manifestPath) {
  $manifest = Get-Content -Raw -Path $manifestPath | ConvertFrom-Json
}
if (-not $Profile) {
  if ($manifest -and $manifest.profile) {
    $Profile = [string]$manifest.profile
  } else {
    $Profile = "cpu-lite"
  }
}
$selectedProfile = Resolve-Profile $Profile

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

$composeArgs = Get-ComposeArgs $selectedProfile
$postgresContainer = Get-ServiceContainer $composeArgs "postgres"
$minioContainer = Get-ServiceContainer $composeArgs "minio"
$qdrantContainer = Get-ServiceContainer $composeArgs "qdrant"
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
    Invoke-Docker ($composeArgs + @("start", "minio", "qdrant", "api", "worker"))
  }
}
