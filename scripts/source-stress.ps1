param(
  [string]$ApiUrl = "http://localhost:8080",
  [string]$TenantId = "",
  [string]$WorkspaceName = "",
  [string]$SourcePath = "",
  [string]$HostFixturePath = "",
  [int]$FileCount = 40,
  [ValidateSet("mixed-docs", "support-ops", "governance")]
  [string]$FixturePreset = "mixed-docs",
  [int]$TimeoutSeconds = 180,
  [string[]]$IncludePatterns = @("**/*"),
  [string[]]$ExcludePatterns = @("archive/**", "drafts/**"),
  [int]$ScanIntervalMinutes = 0,
  [string]$UserId = "",
  [string]$UserEmail = "",
  [switch]$SkipDocumentReady,
  [switch]$KeepSource,
  [switch]$KeepFixture
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $root ".env"
$apiBase = $ApiUrl.TrimEnd("/")
$runId = [Guid]::NewGuid().ToString("N").Substring(0, 12)
$sourceId = ""
$workspaceCreated = $false
$fixtureCreated = $false
$sourceCreated = $false

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

function Read-EnvValue($path, $key) {
  if (-not (Test-Path -LiteralPath $path)) {
    return ""
  }
  foreach ($line in Get-Content -LiteralPath $path) {
    if ($line -match "^\s*$([regex]::Escape($key))=(.*)$") {
      return $matches[1].Trim()
    }
  }
  return ""
}

function Resolve-ApiUrl($url) {
  if ($url -ne "http://localhost:8080") {
    return $url.TrimEnd("/")
  }
  $apiPort = Read-EnvValue $envFile "API_HOST_PORT"
  if ($apiPort -and $apiPort -ne "8080") {
    return "http://localhost:$apiPort"
  }
  return $url.TrimEnd("/")
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

function Join-SourcePath($basePath, $childPath) {
  $trimmedBase = ([string]$basePath).TrimEnd("/", "\")
  $trimmedChild = ([string]$childPath).TrimStart("/", "\")
  if ($trimmedBase -match "^[A-Za-z]:") {
    return (Join-Path $trimmedBase $trimmedChild)
  }
  return "$trimmedBase/$($trimmedChild -replace '\\', '/')"
}

function Resolve-SourceStressPaths {
  $isWindows = [System.IO.Path]::DirectorySeparatorChar -eq "\"
  $sourceHostRoot = Read-EnvValue $envFile "NEXUS_SOURCE_HOST_PATH"
  $sourceContainerRoot = Read-EnvValue $envFile "NEXUS_SOURCE_CONTAINER_PATH"
  if ([string]::IsNullOrWhiteSpace($sourceContainerRoot)) {
    $sourceContainerRoot = "/sources/primary"
  }

  $fixtureLeaf = "nexus-local-source-stress-$runId"
  $resolvedHostPath = $HostFixturePath
  $resolvedSourcePath = $SourcePath

  if ([string]::IsNullOrWhiteSpace($resolvedHostPath)) {
    if (-not [string]::IsNullOrWhiteSpace($sourceHostRoot)) {
      if (-not (Test-Path -LiteralPath $sourceHostRoot)) {
        throw "NEXUS_SOURCE_HOST_PATH does not exist: $sourceHostRoot"
      }
      $resolvedHostPath = Join-Path $sourceHostRoot $fixtureLeaf
      if ([string]::IsNullOrWhiteSpace($resolvedSourcePath)) {
        $resolvedSourcePath = Join-SourcePath $sourceContainerRoot $fixtureLeaf
      }
    } elseif (-not [string]::IsNullOrWhiteSpace($resolvedSourcePath)) {
      if ($resolvedSourcePath.StartsWith("/") -and $isWindows) {
        throw "SourcePath '$resolvedSourcePath' looks container-visible only. Pass -HostFixturePath or set NEXUS_SOURCE_HOST_PATH so the script can generate fixture files."
      }
      $resolvedHostPath = $resolvedSourcePath
    } else {
      $resolvedHostPath = Join-Path ([System.IO.Path]::GetTempPath()) $fixtureLeaf
      $resolvedSourcePath = $resolvedHostPath
    }
  }

  if ([string]::IsNullOrWhiteSpace($resolvedSourcePath)) {
    $resolvedSourcePath = $resolvedHostPath
  }

  return [pscustomobject]@{
    HostFixturePath = [System.IO.Path]::GetFullPath($resolvedHostPath)
    SourcePath = $resolvedSourcePath
  }
}

function Write-StressTextFile($rootPath, $relativePath, $content) {
  $path = Join-Path $rootPath $relativePath
  $parent = Split-Path -Parent $path
  if (-not (Test-Path -LiteralPath $parent)) {
    New-Item -ItemType Directory -Path $parent -Force | Out-Null
  }
  Set-Content -LiteralPath $path -Encoding UTF8 -Value $content
}

function Write-StressBinaryFile($rootPath, $relativePath, [byte[]]$bytes) {
  $path = Join-Path $rootPath $relativePath
  $parent = Split-Path -Parent $path
  if (-not (Test-Path -LiteralPath $parent)) {
    New-Item -ItemType Directory -Path $parent -Force | Out-Null
  }
  [System.IO.File]::WriteAllBytes($path, $bytes)
}

function New-StressDocumentSpec($preset, $index) {
  $number = $index.ToString("0000")
  switch ($preset) {
    "support-ops" {
      switch ($index % 7) {
        0 { return [pscustomobject]@{ RelativePath = "runbooks/oidc/oidc-triage-$number.md"; Topic = "OIDC troubleshooting runbook" } }
        1 { return [pscustomobject]@{ RelativePath = "cases/acme/case-$number.json"; Topic = "Customer support case export" } }
        2 { return [pscustomobject]@{ RelativePath = "kb/sso/article-$number.html"; Topic = "Knowledge base article" } }
        3 { return [pscustomobject]@{ RelativePath = "exports/tickets/ticket-$number.csv"; Topic = "Ticket queue export" } }
        4 { return [pscustomobject]@{ RelativePath = "transcripts/support-call-$number.vtt"; Topic = "Support call transcript" } }
        5 { return [pscustomobject]@{ RelativePath = "cases/beta/notes-$number.txt"; Topic = "Escalation notes" } }
        default { return [pscustomobject]@{ RelativePath = "exports/events/event-$number.tsv"; Topic = "Authentication event export" } }
      }
    }
    "governance" {
      switch ($index % 7) {
        0 { return [pscustomobject]@{ RelativePath = "policies/access/access-policy-$number.md"; Topic = "Access policy" } }
        1 { return [pscustomobject]@{ RelativePath = "processes/intake/intake-flow-$number.html"; Topic = "Workflow procedure" } }
        2 { return [pscustomobject]@{ RelativePath = "records/status/status-export-$number.csv"; Topic = "Paperwork status export" } }
        3 { return [pscustomobject]@{ RelativePath = "requests/customer/request-$number.json"; Topic = "Customer request record" } }
        4 { return [pscustomobject]@{ RelativePath = "audits/evidence/evidence-$number.txt"; Topic = "Audit evidence note" } }
        5 { return [pscustomobject]@{ RelativePath = "training/captions/training-$number.vtt"; Topic = "Training transcript" } }
        default { return [pscustomobject]@{ RelativePath = "records/review/review-$number.tsv"; Topic = "Review queue export" } }
      }
    }
    default {
      $bucket = switch ($index % 4) {
        0 { "runbooks" }
        1 { "cases" }
        2 { "policies" }
        default { "technical-notes" }
      }
      $extension = ".md"
      if ($index % 5 -eq 0) {
        $extension = ".txt"
      }
      return [pscustomobject]@{
        RelativePath = "$bucket/doc-$number$extension"
        Topic = "Source stress document"
      }
    }
  }
}

function New-StressDocumentContent($preset, $spec, $index, $runId, $needle) {
  $extension = [System.IO.Path]::GetExtension($spec.RelativePath).ToLowerInvariant()
  $title = "$($spec.Topic) $index"
  switch ($extension) {
    ".json" {
      return [ordered]@{
        title = $title
        run_id = $runId
        needle = $needle
        status = "open"
        source = $preset
        observations = @(
          "Gateway callback validation succeeded",
          "MFA enrollment was verified",
          "Knowledge base retrieval should find this record"
        )
      } | ConvertTo-Json -Depth 5
    }
    ".html" {
      return @"
<!doctype html>
<html>
<head><meta charset="utf-8"><title>$title</title></head>
<body>
<h1>$title</h1>
<p>Run $runId</p>
<p>Needle: $needle</p>
<p>This page represents a customer knowledge article or process page.</p>
</body>
</html>
"@
    }
    ".csv" {
      return "id,title,needle,status,owner`n$index,""$title"",$needle,open,operations`n"
    }
    ".tsv" {
      return "id`tname`tneedle`tstatus`towner`n$index`t$title`t$needle`tactive`toperations`n"
    }
    ".vtt" {
      return @"
WEBVTT

00:00:00.000 --> 00:00:03.000
$title.

00:00:03.000 --> 00:00:07.000
The trace marker is $needle and the response should be grounded in this transcript.
"@
    }
    default {
      return @"
# $title

Run: $runId
Needle: $needle
Preset: $preset

This fixture file represents realistic customer knowledge that should become searchable after managed source ingestion.
"@
    }
  }
}

function New-SourceStressFixture($path, $fileCount, $runId, $fixturePreset) {
  if ($fileCount -lt 1) {
    throw "FileCount must be at least 1."
  }
  if (Test-Path -LiteralPath $path) {
    $existing = @(Get-ChildItem -LiteralPath $path -Force)
    if ($existing.Count -gt 0) {
      throw "Fixture path already exists and is not empty: $path"
    }
  } else {
    New-Item -ItemType Directory -Path $path -Force | Out-Null
  }

  $imported = @()
  for ($index = 1; $index -le $fileCount; $index++) {
    $spec = New-StressDocumentSpec $fixturePreset $index
    $relative = $spec.RelativePath
    $needle = "nexus-source-stress-$runId-$($index.ToString('0000'))"
    $content = New-StressDocumentContent $fixturePreset $spec $index $runId $needle
    Write-StressTextFile $path $relative $content
    $imported += [pscustomobject]@{
      RelativePath = ($relative -replace "\\", "/")
      Needle = $needle
    }
  }

  Write-StressBinaryFile $path "assets/image.png" ([byte[]](0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A))
  Write-StressBinaryFile $path "assets/raw.bin" ([byte[]](0x00, 0x01, 0x02, 0x03, 0x04))
  Write-StressTextFile $path "assets/notes.tmp" "Unsupported temporary file for $runId"
  Write-StressTextFile $path "archive/old.md" "Excluded archive file for $runId"
  Write-StressTextFile $path "drafts/draft.md" "Excluded draft file for $runId"
  Write-StressTextFile $path ".hidden.md" "Hidden file for $runId"
  Write-StressTextFile $path "node_modules/package.md" "Policy skipped cache file for $runId"

  return [pscustomobject]@{
    Imported = $imported
    Preset = $fixturePreset
    ExpectedImported = $fileCount
    ExpectedSkipped = 7
    ExpectedFailed = 0
    ExpectedReasons = [ordered]@{
      unsupported_type = 3
      excluded = 2
      policy = 2
    }
  }
}

function Select-TenantId($workspaceName) {
  if (-not [string]::IsNullOrWhiteSpace($TenantId)) {
    return $TenantId
  }

  $me = Invoke-Json "GET" "$apiBase/v1/me"
  $memberships = @($me.memberships)
  if (-not [string]::IsNullOrWhiteSpace($workspaceName)) {
    $match = $memberships | Where-Object { $_.tenant.name -eq $workspaceName } | Select-Object -First 1
    if ($match) {
      Pass "workspace" "$($match.tenant.name) -> $($match.tenant.id)"
      return $match.tenant.id
    }
  }

  if ([string]::IsNullOrWhiteSpace($workspaceName)) {
    $workspaceName = "Nexus Source Stress $runId"
  }
  $workspace = Invoke-Json "POST" "$apiBase/v1/tenants" @{
    name = $workspaceName
  }
  $script:workspaceCreated = $true
  Pass "workspace" "$workspaceName -> $($workspace.tenant.id)"
  return $workspace.tenant.id
}

function Wait-SourceScan($tenantId, $sourceId, $jobId) {
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  $lastState = "queued"
  $detail = $null

  while ((Get-Date) -lt $deadline) {
    $encodedTenant = UrlEncode $tenantId
    $encodedSource = UrlEncode $sourceId
    $detail = Invoke-Json "GET" "$apiBase/v1/data-sources/$encodedSource`?tenant_id=$encodedTenant&scan_entry_limit=500"
    $job = @($detail.jobs | Where-Object { $_.id -eq $jobId }) | Select-Object -First 1
    if ($job) {
      $lastState = $job.state
      if ($job.state -in @("succeeded", "failed", "canceled")) {
        return $detail
      }
    } elseif ($detail.source.status -in @("active", "failed")) {
      return $detail
    }
    Start-Sleep -Seconds 2
  }

  throw "source scan $jobId did not finish before timeout; last state: $lastState"
}

function Get-SourceScanEntries($tenantId, $sourceId, $outcome) {
  $entries = @()
  $offset = 0
  $limit = 500
  $encodedTenant = UrlEncode $tenantId
  $encodedSource = UrlEncode $sourceId

  while ($true) {
    $url = "$apiBase/v1/data-sources/$encodedSource`?tenant_id=$encodedTenant&scan_entry_limit=$limit&scan_entry_offset=$offset"
    if (-not [string]::IsNullOrWhiteSpace($outcome)) {
      $url = "$url&scan_entry_outcome=$(UrlEncode $outcome)"
    }
    $detail = Invoke-Json "GET" $url
    $pageEntries = @($detail.scan_entries)
    $entries += $pageEntries
    if (-not $detail.scan_entries_page.has_more) {
      return $entries
    }
    if ($pageEntries.Count -eq 0) {
      throw "scan entry paging made no progress at offset $offset"
    }
    $offset += $detail.scan_entries_page.limit
  }
}

function Wait-ImportedDocumentsReady($tenantId, $documentIds) {
  $ids = @($documentIds | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -Unique)
  if ($ids.Count -eq 0) {
    throw "source scan produced no imported document IDs"
  }

  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  $pending = [System.Collections.Generic.HashSet[string]]::new()
  foreach ($id in $ids) {
    [void]$pending.Add([string]$id)
  }
  $lastPending = $pending.Count

  while ((Get-Date) -lt $deadline) {
    foreach ($id in @($pending)) {
      $encodedTenant = UrlEncode $tenantId
      $encodedDocument = UrlEncode $id
      $detail = Invoke-Json "GET" "$apiBase/v1/documents/$encodedDocument`?tenant_id=$encodedTenant"
      if ($detail.document.status -eq "ready") {
        [void]$pending.Remove($id)
      } elseif ($detail.document.status -eq "failed") {
        $jobSummary = @($detail.jobs) | ForEach-Object { "$($_.id):$($_.state):$($_.error_message)" }
        throw "document ingestion failed for $id; jobs: $($jobSummary -join ', ')"
      }
    }

    if ($pending.Count -eq 0) {
      return
    }
    if ($pending.Count -ne $lastPending) {
      Write-Host "[WAIT] documents ready - $($ids.Count - $pending.Count)/$($ids.Count)"
      $lastPending = $pending.Count
    }
    Start-Sleep -Seconds 2
  }

  throw "$($pending.Count) imported document(s) did not become ready before timeout"
}

function Assert-SearchHit($tenantId, $documentId, $needle) {
  $search = Invoke-Json "POST" "$apiBase/v1/search" @{
    tenant_id = $tenantId
    document_id = $documentId
    query = $needle
    limit = 5
  }
  $hits = @($search.hits)
  $match = $hits | Where-Object {
    $_.document_id -eq $documentId -and $_.text -like "*$needle*"
  } | Select-Object -First 1
  if (-not $match) {
    throw "search did not return the expected source-stress hit for $documentId"
  }
  return $hits.Count
}

function Assert-ScanSummary($detail, $expected) {
  $summary = $detail.scan_summary
  if ($summary.imported -ne $expected.ExpectedImported) {
    throw "scan imported $($summary.imported), want $($expected.ExpectedImported)"
  }
  if ($summary.skipped -ne $expected.ExpectedSkipped) {
    throw "scan skipped $($summary.skipped), want $($expected.ExpectedSkipped)"
  }
  if ($summary.failed -ne $expected.ExpectedFailed) {
    throw "scan failed $($summary.failed), want $($expected.ExpectedFailed)"
  }
  foreach ($reason in $expected.ExpectedReasons.Keys) {
    $actual = 0
    if ($summary.reasons.PSObject.Properties.Name -contains $reason) {
      $actual = [int]$summary.reasons.$reason
    }
    if ($actual -ne $expected.ExpectedReasons[$reason]) {
      throw "scan reason '$reason' count $actual, want $($expected.ExpectedReasons[$reason])"
    }
  }
}

try {
  $apiBase = Resolve-ApiUrl $ApiUrl
  $paths = Resolve-SourceStressPaths
  $fixtureRoot = $paths.HostFixturePath
  $sourceRoot = $paths.SourcePath

  Write-Host "Nexus Local source stress test"
  Write-Host "API:        $apiBase"
  Write-Host "Host path:  $fixtureRoot"
  Write-Host "Source:     $sourceRoot"
  Write-Host "Preset:     $FixturePreset"
  Write-Host "Files:      $FileCount supported + mixed skipped fixtures"
  Write-Host ""

  $health = Invoke-Json "GET" "$apiBase/healthz"
  Pass "api health" "$($health.status) $($health.version)"

  $readiness = Invoke-Json "GET" "$apiBase/readyz"
  Pass "api readiness" "persistence=$($readiness.persistence_backend), objects=$($readiness.object_storage_backend), vectors=$($readiness.vector_backend), embeddings=$($readiness.embedding_backend)"

  $tenantId = Select-TenantId $WorkspaceName

  $expected = New-SourceStressFixture $fixtureRoot $FileCount $runId $FixturePreset
  $fixtureCreated = $true
  Pass "fixture" "$($expected.Preset): $($expected.ExpectedImported) importable, $($expected.ExpectedSkipped) expected skips"

  $source = Invoke-Json "POST" "$apiBase/v1/data-sources" @{
    tenant_id = $tenantId
    type = "folder"
    name = "Nexus Source Stress $FixturePreset $runId"
    root_path = $sourceRoot
    include_patterns = $IncludePatterns
    exclude_patterns = $ExcludePatterns
    scan_interval_minutes = $ScanIntervalMinutes
  }
  $sourceId = $source.source.id
  $sourceCreated = $true
  Pass "source" "$($source.source.name) -> $sourceId"

  $encodedTenantForScan = UrlEncode $tenantId
  $encodedSourceForScan = UrlEncode $sourceId
  $scan = Invoke-Json "POST" "$apiBase/v1/data-sources/$encodedSourceForScan/scan?tenant_id=$encodedTenantForScan"
  $scanJobId = $scan.job.id
  Pass "scan queued" $scanJobId

  $detail = Wait-SourceScan $tenantId $sourceId $scanJobId
  $scanJob = @($detail.jobs | Where-Object { $_.id -eq $scanJobId }) | Select-Object -First 1
  if ($scanJob -and $scanJob.state -ne "succeeded") {
    throw "source scan ended in $($scanJob.state): $($scanJob.error_message)"
  }
  Assert-ScanSummary $detail $expected
  Pass "scan summary" "$($detail.scan_summary.imported) imported, $($detail.scan_summary.skipped) skipped, $($detail.scan_summary.failed) failed"

  $importedEntries = @(Get-SourceScanEntries $tenantId $sourceId "imported")
  if ($importedEntries.Count -ne $expected.ExpectedImported) {
    throw "detail returned $($importedEntries.Count) imported entries, want $($expected.ExpectedImported)"
  }

  if ($SkipDocumentReady) {
    Warn "document readiness" "skipped by request"
  } else {
    Wait-ImportedDocumentsReady $tenantId (@($importedEntries | ForEach-Object { $_.document_id }))
    Pass "document readiness" "$($importedEntries.Count) ready"

    $firstExpected = @($expected.Imported)[0]
    $firstEntry = @($importedEntries | Where-Object { $_.path -eq $firstExpected.RelativePath }) | Select-Object -First 1
    if (-not $firstEntry) {
      throw "imported scan entry missing for $($firstExpected.RelativePath)"
    }
    $hitCount = Assert-SearchHit $tenantId $firstEntry.document_id $firstExpected.Needle
    Pass "search" "$hitCount hit(s) for $($firstExpected.RelativePath)"
  }

  Write-Host ""
  Pass "source stress" "workspace=$tenantId source=$sourceId job=$scanJobId"
} finally {
  if ($sourceCreated -and -not $KeepSource) {
    try {
      $encodedTenant = UrlEncode $tenantId
      $encodedSource = UrlEncode $sourceId
      Invoke-Json "DELETE" "$apiBase/v1/data-sources/$encodedSource`?tenant_id=$encodedTenant&delete_documents=true" | Out-Null
      Pass "cleanup source" "archived $sourceId and deleted source documents"
    } catch {
      Warn "cleanup source" $_.Exception.Message
    }
  }

  if ($workspaceCreated) {
    Warn "cleanup workspace" "left $tenantId; workspace deletion is not implemented yet"
  }

  if ($fixtureCreated -and -not $KeepFixture) {
    try {
      $leaf = Split-Path -Leaf $fixtureRoot
      if ($leaf -like "nexus-local-source-stress-*") {
        Remove-Item -LiteralPath $fixtureRoot -Recurse -Force
        Pass "cleanup fixture" $fixtureRoot
      } else {
        Warn "cleanup fixture" "kept $fixtureRoot because it does not look like a generated stress folder"
      }
    } catch {
      Warn "cleanup fixture" $_.Exception.Message
    }
  }
}
