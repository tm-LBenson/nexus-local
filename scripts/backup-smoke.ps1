param(
  [string]$Profile = "",
  [string]$OutputDir = "",
  [string]$Name = "",
  [string]$HelperImage = "alpine:3.20",
  [string]$ApiUrl = "http://localhost:8080",
  [string]$UserId = "",
  [string]$UserEmail = "",
  [int]$TimeoutSeconds = 120,
  [switch]$SkipServiceStop,
  [switch]$KeepBackup,
  [switch]$RestoreRoundTrip
)

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Net.Http

$root = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $root ".env"
$composeHelper = Join-Path (Join-Path $PSScriptRoot "lib") "compose.ps1"
. $composeHelper

$requestHeaders = @{}
if (-not [string]::IsNullOrWhiteSpace($UserId)) {
  $requestHeaders["X-User-ID"] = $UserId.Trim()
}
if (-not [string]::IsNullOrWhiteSpace($UserEmail)) {
  $requestHeaders["X-User-Email"] = $UserEmail.Trim()
}

function Pass($name, $detail = "") {
  if ($detail) {
    Write-Host "[OK]   $name - $detail"
  } else {
    Write-Host "[OK]   $name"
  }
}

function Warn($name, $detail = "") {
  if ($detail) {
    Write-Host "[WARN] $name - $detail"
  } else {
    Write-Host "[WARN] $name"
  }
}

function UrlEncode($value) {
  return [System.Uri]::EscapeDataString([string]$value)
}

function Read-WebError($errorRecord) {
  if ($errorRecord.ErrorDetails -and $errorRecord.ErrorDetails.Message) {
    return "$($errorRecord.Exception.Message): $($errorRecord.ErrorDetails.Message)"
  }

  $response = $errorRecord.Exception.Response
  if (-not $response) {
    return $errorRecord.Exception.Message
  }

  try {
    $stream = $response.GetResponseStream()
    if (-not $stream) {
      return $errorRecord.Exception.Message
    }
    $reader = [System.IO.StreamReader]::new($stream)
    $body = $reader.ReadToEnd()
    if ($body) {
      return "$($errorRecord.Exception.Message): $body"
    }
  } catch {
    return $errorRecord.Exception.Message
  }

  return $errorRecord.Exception.Message
}

function Invoke-Json($method, $url, $body = $null) {
  $params = @{
    Method = $method
    Uri = $url
    TimeoutSec = 20
  }

  if ($requestHeaders.Count -gt 0) {
    $params.Headers = $requestHeaders
  }

  if ($null -ne $body) {
    $params.ContentType = "application/json"
    $params.Body = $body | ConvertTo-Json -Depth 10 -Compress
  }

  try {
    return Invoke-RestMethod @params
  } catch {
    throw "$(Read-WebError $_)"
  }
}

function Invoke-DocumentUpload($url, $tenantId, $path) {
  $client = [System.Net.Http.HttpClient]::new()
  $content = [System.Net.Http.MultipartFormDataContent]::new()
  $stream = $null

  try {
    foreach ($key in $requestHeaders.Keys) {
      $client.DefaultRequestHeaders.Remove($key) | Out-Null
      $client.DefaultRequestHeaders.Add($key, [string]$requestHeaders[$key])
    }

    $content.Add([System.Net.Http.StringContent]::new($tenantId), "tenant_id")

    $stream = [System.IO.File]::OpenRead($path)
    $fileContent = [System.Net.Http.StreamContent]::new($stream)
    $fileContent.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::Parse("text/markdown")
    $content.Add($fileContent, "file", [System.IO.Path]::GetFileName($path))

    $response = $client.PostAsync($url, $content).GetAwaiter().GetResult()
    $responseBody = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
    if (-not $response.IsSuccessStatusCode) {
      throw "upload failed: HTTP $([int]$response.StatusCode) $responseBody"
    }
    return $responseBody | ConvertFrom-Json
  } finally {
    if ($content) {
      $content.Dispose()
    }
    if ($stream) {
      $stream.Dispose()
    }
    $client.Dispose()
  }
}

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

function Wait-ApiReady($apiBase) {
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  $lastError = ""

  while ((Get-Date) -lt $deadline) {
    try {
      $health = Invoke-Json "GET" "$apiBase/healthz"
      $ready = Invoke-Json "GET" "$apiBase/readyz"
      if ($health.status -and $ready.persistence_backend) {
        return
      }
    } catch {
      $lastError = $_.Exception.Message
    }
    Start-Sleep -Seconds 2
  }

  throw "API did not become ready before timeout. $lastError"
}

function New-RoundTripDocument($outputPath, $runId, $needle) {
  $content = @"
# Nexus Local backup restore smoke

This document exists only to prove backup and restore round trips.

Round trip id: $runId
Unique restore phrase: $needle
"@
  Set-Content -LiteralPath $outputPath -Encoding UTF8 -Value $content
}

function Wait-DocumentReady($apiBase, $tenantId, $documentId) {
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  $lastStatus = "unknown"
  $detail = $null

  while ((Get-Date) -lt $deadline) {
    $encodedTenant = UrlEncode $tenantId
    $encodedDocument = UrlEncode $documentId
    $detail = Invoke-Json "GET" "$apiBase/v1/documents/$encodedDocument`?tenant_id=$encodedTenant"
    $lastStatus = $detail.document.status

    if ($lastStatus -eq "ready") {
      return $detail
    }
    if ($lastStatus -eq "failed") {
      $jobSummary = @($detail.jobs) | ForEach-Object { "$($_.id):$($_.state)" }
      throw "document ingestion failed for $documentId; jobs: $($jobSummary -join ', ')"
    }

    Start-Sleep -Seconds 2
  }

  throw "document $documentId did not become ready before timeout; last status: $lastStatus"
}

function Assert-SearchRestored($apiBase, $tenantId, $documentId, $needle, $label = "restore search") {
  $search = Invoke-Json "POST" "$apiBase/v1/search" @{
    tenant_id = $tenantId
    document_id = $documentId
    query = $needle
    limit = 5
  }
  $hits = @($search.hits)
  $matchingHits = @($hits | Where-Object { $_.document_id -eq $documentId -and $_.text -like "*$needle*" })
  if ($matchingHits.Count -lt 1) {
    throw "restored search did not return $documentId with the round-trip phrase"
  }
  Pass $label "$($matchingHits.Count) matching hit(s)"
}

function Assert-DownloadRestored($apiBase, $tenantId, $documentId, $needle, $label = "restore download") {
  $encodedTenant = UrlEncode $tenantId
  $encodedDocument = UrlEncode $documentId
  $params = @{
    Method = "GET"
    Uri = "$apiBase/v1/documents/$encodedDocument/download?tenant_id=$encodedTenant"
    TimeoutSec = 20
  }
  if ($requestHeaders.Count -gt 0) {
    $params.Headers = $requestHeaders
  }
  try {
    $content = Invoke-RestMethod @params
  } catch {
    throw "$(Read-WebError $_)"
  }
  if ([string]$content -notlike "*$needle*") {
    throw "restored download did not contain the round-trip phrase"
  }
  Pass $label "object content matched"
}

function New-RoundTripFixture($apiBase) {
  $runId = [Guid]::NewGuid().ToString("N").Substring(0, 12)
  $workspaceName = "Nexus Backup Restore $runId"
  $documentName = "nexus-local-backup-restore-$runId.md"
  $needle = "nexus-local-restore-needle-$runId"
  $tempPath = Join-Path ([System.IO.Path]::GetTempPath()) $documentName
  $tenantId = ""

  try {
    New-RoundTripDocument $tempPath $runId $needle
    $workspace = Invoke-Json "POST" "$apiBase/v1/tenants" @{
      name = $workspaceName
    }
    $tenantId = $workspace.tenant.id
    Pass "round-trip workspace" "$workspaceName -> $tenantId"

    $upload = Invoke-DocumentUpload "$apiBase/v1/documents/upload" $tenantId $tempPath
    $documentId = $upload.document.id
    Pass "round-trip upload" "$documentName -> $documentId"

    Wait-DocumentReady $apiBase $tenantId $documentId | Out-Null
    Pass "round-trip ingestion" "document ready"
    Assert-SearchRestored $apiBase $tenantId $documentId $needle "round-trip search"
    Assert-DownloadRestored $apiBase $tenantId $documentId $needle "round-trip download"

    return [pscustomobject]@{
      TenantId = $tenantId
      DocumentId = $documentId
      Needle = $needle
      WorkspaceName = $workspaceName
    }
  } catch {
    if (-not [string]::IsNullOrWhiteSpace($tenantId)) {
      Remove-RoundTripWorkspace $apiBase $tenantId
    }
    throw
  } finally {
    if (Test-Path -LiteralPath $tempPath) {
      Remove-Item -LiteralPath $tempPath -Force
    }
  }
}

function Remove-RoundTripWorkspace($apiBase, $tenantId, [switch]$Required) {
  if ([string]::IsNullOrWhiteSpace($tenantId)) {
    return
  }
  $encodedTenant = UrlEncode $tenantId
  try {
    $documents = Invoke-Json "GET" "$apiBase/v1/documents?tenant_id=$encodedTenant"
    foreach ($document in @($documents.documents)) {
      $encodedDocument = UrlEncode $document.id
      Invoke-Json "DELETE" "$apiBase/v1/documents/$encodedDocument`?tenant_id=$encodedTenant" | Out-Null
      Pass "cleanup document" "deleted $($document.id)"
    }
  } catch {
    if ($Required) {
      throw "cleanup documents: $($_.Exception.Message)"
    } else {
      Warn "cleanup documents" $_.Exception.Message
    }
  }
  try {
    Invoke-Json "DELETE" "$apiBase/v1/tenants/$encodedTenant" | Out-Null
    Pass "cleanup workspace" "deleted $tenantId"
  } catch {
    if ($Required) {
      throw "cleanup workspace: $($_.Exception.Message)"
    } else {
      Warn "cleanup workspace" $_.Exception.Message
    }
  }
}

$selectedProfile = Resolve-NexusDeploymentProfile -Profile $Profile -EnvFile $envFile
$apiHostPort = Read-NexusEnvValue $envFile "API_HOST_PORT"
if (-not $apiHostPort) {
  $apiHostPort = "8080"
}
if ($ApiUrl -eq "http://localhost:8080" -and $apiHostPort -ne "8080") {
  $ApiUrl = "http://localhost:$apiHostPort"
}
$apiBase = $ApiUrl.TrimEnd("/")
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
if ($RestoreRoundTrip) {
  Write-Host "Restore round trip: $apiBase"
}
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

$roundTrip = $null
try {
  if ($RestoreRoundTrip) {
    Wait-ApiReady $apiBase
    $roundTrip = New-RoundTripFixture $apiBase
  }

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

  if ($RestoreRoundTrip) {
    Write-Host ""
    Write-Host "Mutating round-trip data before restore..."
    Remove-RoundTripWorkspace $apiBase $roundTrip.TenantId -Required

    $restoreLiveArgs = @{
      BackupPath = $backupPath
      Profile = $selectedProfile
      HelperImage = $HelperImage
      Force = $true
    }
    & $restoreScript @restoreLiveArgs
    if ($LASTEXITCODE -ne 0) {
      throw "restore.ps1 round trip failed with exit code $LASTEXITCODE"
    }

    Wait-ApiReady $apiBase
    Wait-DocumentReady $apiBase $roundTrip.TenantId $roundTrip.DocumentId | Out-Null
    Pass "restore document" "$($roundTrip.DocumentId) is ready"
    Assert-SearchRestored $apiBase $roundTrip.TenantId $roundTrip.DocumentId $roundTrip.Needle
    Assert-DownloadRestored $apiBase $roundTrip.TenantId $roundTrip.DocumentId $roundTrip.Needle
    Remove-RoundTripWorkspace $apiBase $roundTrip.TenantId -Required
    $roundTrip = $null
  }

  Write-Host ""
  Write-Host "[OK] Backup smoke passed."
} finally {
  if ($roundTrip) {
    Remove-RoundTripWorkspace $apiBase $roundTrip.TenantId
  }
  if (-not $KeepBackup -and (Test-Path $backupPath)) {
    Remove-Item -LiteralPath $backupPath -Recurse -Force
    Write-Host "Removed temporary backup: $backupPath"
  } elseif ($KeepBackup -and (Test-Path $backupPath)) {
    Write-Host "Kept backup: $backupPath"
  }
}
