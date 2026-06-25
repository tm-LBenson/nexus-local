param(
  [string]$Profile = "cpu-lite",
  [string]$OutputDir = "",
  [string]$Name = "",
  [string]$HelperImage = "alpine:3.20",
  [switch]$SkipServiceStop
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $root ".env"
$profileNames = @("cpu-lite", "split-nas-gpu", "gpu-local", "prod-auth")

if (-not $OutputDir) {
  $OutputDir = Join-Path $root "backups"
}

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
    throw "Container for service '$service' was not found. Start the selected Compose profile before backing up."
  }
  return $id
}

function Backup-VolumeFromContainer($containerID, $sourcePath, $archiveName, $backupPath) {
  Write-Host "Backing up $archiveName..."
  Invoke-Docker @(
    "run", "--rm",
    "--volumes-from", $containerID,
    "-v", "$backupPath`:/backup",
    $HelperImage,
    "sh", "-lc", "cd $sourcePath && tar -czf /backup/$archiveName ."
  )
}

$selectedProfile = Resolve-Profile $Profile
Require-Command docker

$composeArgs = Get-ComposeArgs $selectedProfile
$postgresContainer = Get-ServiceContainer $composeArgs "postgres"
$minioContainer = Get-ServiceContainer $composeArgs "minio"
$qdrantContainer = Get-ServiceContainer $composeArgs "qdrant"

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
if (-not $Name) {
  $Name = "nexus-local-$selectedProfile-$timestamp"
}
$safeName = $Name -replace '[^A-Za-z0-9_.-]', '-'
$outputRoot = [System.IO.Path]::GetFullPath($OutputDir)
$backupPath = Join-Path $outputRoot $safeName
if (Test-Path $backupPath) {
  throw "Backup path already exists: $backupPath"
}
New-Item -ItemType Directory -Path $backupPath | Out-Null

$stoppedServices = @()
try {
  if (-not $SkipServiceStop) {
    $stoppedServices = @("api", "worker", "minio", "qdrant")
    Write-Host "Pausing app writes and cold-copy services..."
    Invoke-Docker ($composeArgs + @("stop") + $stoppedServices)
  }

  Write-Host "Backing up postgres.dump..."
  Invoke-Docker ($composeArgs + @(
      "exec", "-T", "postgres", "sh", "-lc",
      'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc -f /tmp/nexus-local-postgres.dump'
    ))
  Invoke-Docker ($composeArgs + @(
      "cp", "postgres:/tmp/nexus-local-postgres.dump",
      (Join-Path $backupPath "postgres.dump")
    ))
  Invoke-Docker ($composeArgs + @(
      "exec", "-T", "postgres", "rm", "-f", "/tmp/nexus-local-postgres.dump"
    ))

  Backup-VolumeFromContainer $minioContainer "/data" "minio-data.tgz" $backupPath
  Backup-VolumeFromContainer $qdrantContainer "/qdrant/storage" "qdrant-storage.tgz" $backupPath

  if (Test-Path $envFile) {
    Copy-Item -LiteralPath $envFile -Destination (Join-Path $backupPath "env.snapshot") -Force
  }

  $gitCommit = ""
  if (Get-Command git -ErrorAction SilentlyContinue) {
    try {
      $gitCommit = (& git -C $root rev-parse HEAD).Trim()
    } catch {
      $gitCommit = ""
    }
  }
  $envSnapshot = ""
  if (Test-Path (Join-Path $backupPath "env.snapshot")) {
    $envSnapshot = "env.snapshot"
  }

  $manifest = [ordered]@{
    version = 1
    created_at = (Get-Date).ToUniversalTime().ToString("o")
    profile = $selectedProfile
    git_commit = $gitCommit
    components = [ordered]@{
      postgres = "postgres.dump"
      minio = "minio-data.tgz"
      qdrant = "qdrant-storage.tgz"
      env = $envSnapshot
    }
  }
  $manifest | ConvertTo-Json -Depth 4 | Set-Content -Path (Join-Path $backupPath "manifest.json") -Encoding ascii

  Write-Host ""
  Write-Host "Backup complete: $backupPath"
} finally {
  if ($stoppedServices.Count -gt 0) {
    Write-Host "Restarting paused services..."
    Invoke-Docker ($composeArgs + @("start") + $stoppedServices)
  }
}
