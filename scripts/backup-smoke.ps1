param(
  [string]$Profile = "",
  [string]$OutputDir = "",
  [string]$Name = "",
  [string]$HelperImage = "alpine:3.20",
  [switch]$SkipServiceStop,
  [switch]$KeepBackup
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $root ".env"
$composeHelper = Join-Path (Join-Path $PSScriptRoot "lib") "compose.ps1"
. $composeHelper

function Invoke-Docker($arguments) {
  & docker @arguments
  if ($LASTEXITCODE -ne 0) {
    throw "docker $($arguments -join ' ') failed"
  }
}

function Require-File($path) {
  if (-not (Test-Path $path)) {
    throw "Expected backup file is missing: $path"
  }
  $item = Get-Item -LiteralPath $path
  if ($item.Length -le 0) {
    throw "Expected backup file is empty: $path"
  }
  return $item
}

function Test-TarArchive($backupPath, $archiveName, $helperImage) {
  Invoke-Docker @(
    "run", "--rm",
    "-v", "$backupPath`:/backup:ro",
    $helperImage,
    "sh", "-lc",
    "tar -tzf /backup/$archiveName >/dev/null"
  )
}

function Test-PostgresDump($composeArgs, $backupPath) {
  $tempDump = "/tmp/nexus-local-backup-smoke.dump"
  try {
    Invoke-Docker ($composeArgs + @("cp", (Join-Path $backupPath "postgres.dump"), "postgres:$tempDump"))
    Invoke-Docker ($composeArgs + @("exec", "-T", "postgres", "sh", "-lc", "pg_restore -l $tempDump >/dev/null"))
  } finally {
    try {
      Invoke-Docker ($composeArgs + @("exec", "-T", "postgres", "rm", "-f", $tempDump))
    } catch {
      Write-Host "[WARN] Could not remove temporary dump from postgres: $($_.Exception.Message)"
    }
  }
}

$selectedProfile = Resolve-NexusDeploymentProfile -Profile $Profile -EnvFile $envFile
if (-not $OutputDir) {
  $OutputDir = Join-Path (Join-Path $root "tmp") "backup-smoke"
}
if (-not $Name) {
  $Name = "nexus-local-backup-smoke-$selectedProfile-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
}
$safeName = $Name -replace '[^A-Za-z0-9_.-]', '-'
if ([string]::IsNullOrWhiteSpace($safeName) -or $safeName -in @(".", "..")) {
  throw "Backup smoke name must resolve to a child folder name."
}
$outputRoot = [System.IO.Path]::GetFullPath($OutputDir)
$backupPath = [System.IO.Path]::GetFullPath((Join-Path $outputRoot $safeName))
$outputRootWithSeparator = $outputRoot.TrimEnd(
  [System.IO.Path]::DirectorySeparatorChar,
  [System.IO.Path]::AltDirectorySeparatorChar
) + [System.IO.Path]::DirectorySeparatorChar
if (-not $backupPath.StartsWith($outputRootWithSeparator, [System.StringComparison]::OrdinalIgnoreCase)) {
  throw "Backup smoke path must stay inside the output directory."
}

Write-Host "Nexus Local backup smoke"
Write-Host "Profile: $selectedProfile"
Write-Host "Backup path: $backupPath"
Write-Host ""

$backupArgs = @{
  Profile = $selectedProfile
  OutputDir = $outputRoot
  Name = $safeName
  HelperImage = $HelperImage
}
if ($SkipServiceStop) {
  $backupArgs.SkipServiceStop = $true
}

try {
  $backupScript = Join-Path $PSScriptRoot "backup.ps1"
  & $backupScript @backupArgs
  if ($LASTEXITCODE -ne 0) {
    throw "backup.ps1 failed with exit code $LASTEXITCODE"
  }

  $manifestFile = Require-File (Join-Path $backupPath "manifest.json")
  $postgresDump = Require-File (Join-Path $backupPath "postgres.dump")
  $minioArchive = Require-File (Join-Path $backupPath "minio-data.tgz")
  $qdrantArchive = Require-File (Join-Path $backupPath "qdrant-storage.tgz")
  $manifest = Get-Content -Raw -Path $manifestFile.FullName | ConvertFrom-Json
  if ([int]$manifest.version -ne 1) {
    throw "Unexpected backup manifest version: $($manifest.version)"
  }
  if ([string]$manifest.profile -ne $selectedProfile) {
    throw "Backup manifest profile '$($manifest.profile)' did not match '$selectedProfile'"
  }
  if ($manifest.components.postgres -ne "postgres.dump" -or
      $manifest.components.minio -ne "minio-data.tgz" -or
      $manifest.components.qdrant -ne "qdrant-storage.tgz") {
    throw "Backup manifest component names are not recognized."
  }

  $composeConfig = Get-NexusComposeConfig -Root $root -EnvFile $envFile -Profile $selectedProfile
  Test-PostgresDump $composeConfig.Args $backupPath
  Test-TarArchive $backupPath $minioArchive.Name $HelperImage
  Test-TarArchive $backupPath $qdrantArchive.Name $HelperImage

  $restoreScript = Join-Path $PSScriptRoot "restore.ps1"
  $restoreArgs = @{
    BackupPath = $backupPath
    Profile = $selectedProfile
    HelperImage = $HelperImage
    ValidateOnly = $true
  }
  & $restoreScript @restoreArgs
  if ($LASTEXITCODE -ne 0) {
    throw "restore.ps1 validation failed with exit code $LASTEXITCODE"
  }

  Write-Host ""
  Write-Host "[OK] Backup smoke passed."
} finally {
  if (-not $KeepBackup -and (Test-Path $backupPath)) {
    Remove-Item -LiteralPath $backupPath -Recurse -Force
    Write-Host "Removed temporary backup: $backupPath"
  } elseif ($KeepBackup -and (Test-Path $backupPath)) {
    Write-Host "Kept backup: $backupPath"
  }
}
