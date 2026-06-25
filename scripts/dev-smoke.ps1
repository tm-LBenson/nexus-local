param(
  [string]$ApiUrl = "http://localhost:8080",
  [string]$TenantId = "tenant_1",
  [int]$TimeoutSeconds = 90,
  [switch]$IncludeAsk,
  [switch]$KeepDocument
)

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Net.Http

$apiBase = $ApiUrl.TrimEnd("/")
$runId = [Guid]::NewGuid().ToString("N").Substring(0, 12)
$documentName = "nexus-local-smoke-$runId.txt"
$needle = "nexus local smoke needle $runId"
$tempPath = Join-Path ([System.IO.Path]::GetTempPath()) $documentName
$documentId = $null

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

function Read-WebError($errorRecord) {
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
    TimeoutSec = 15
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
    $content.Add([System.Net.Http.StringContent]::new($tenantId), "tenant_id")

    $stream = [System.IO.File]::OpenRead($path)
    $fileContent = [System.Net.Http.StreamContent]::new($stream)
    $fileContent.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::Parse("text/plain")
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

function Wait-DocumentReady($documentId) {
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  $lastStatus = "unknown"
  $detail = $null

  while ((Get-Date) -lt $deadline) {
    $detail = Invoke-Json "GET" "$apiBase/v1/documents/$documentId`?tenant_id=$TenantId"
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

try {
  Write-Host "Nexus Local smoke test"
  Write-Host ""

  $health = Invoke-Json "GET" "$apiBase/healthz"
  Pass "api health" "$($health.status) $($health.version)"

  $readiness = Invoke-Json "GET" "$apiBase/readyz"
  Pass "api readiness" "persistence=$($readiness.persistence_backend), objects=$($readiness.object_storage_backend), vectors=$($readiness.vector_backend), embeddings=$($readiness.embedding_backend)"

  Set-Content -LiteralPath $tempPath -Encoding UTF8 -Value @"
Nexus Local smoke document.

This file verifies upload, worker ingestion, vector indexing, and search.
Unique phrase: $needle
"@

  $upload = Invoke-DocumentUpload "$apiBase/v1/documents/upload" $TenantId $tempPath
  $documentId = $upload.document.id
  Pass "upload" "$documentName -> $documentId"

  $detail = Wait-DocumentReady $documentId
  $latestJob = @($detail.jobs) | Select-Object -First 1
  if ($latestJob) {
    Pass "ingestion" "document ready, latest job=$($latestJob.state)"
  } else {
    Pass "ingestion" "document ready"
  }

  $search = Invoke-Json "POST" "$apiBase/v1/search" @{
    tenant_id = $TenantId
    document_id = $documentId
    query = $needle
    limit = 5
  }
  $hits = @($search.hits)
  if ($hits.Count -lt 1) {
    throw "search returned no hits for smoke document"
  }
  $matchingHits = @($hits | Where-Object { $_.text -like "*$needle*" })
  if ($matchingHits.Count -lt 1) {
    throw "search returned hits, but none contained the smoke phrase"
  }
  Pass "search" "$($hits.Count) hit(s), first score=$($hits[0].score)"

  if ($IncludeAsk) {
    $ask = Invoke-Json "POST" "$apiBase/v1/conversations/ask" @{
      tenant_id = $TenantId
      document_id = $documentId
      question = "What is the unique smoke phrase?"
      limit = 5
    }
    if (-not $ask.answer -and -not $ask.assistant_message.content) {
      throw "ask completed but returned no assistant content"
    }
    Pass "ask" "conversation=$($ask.conversation.id)"
  } else {
    Warn "ask" "skipped; rerun with -IncludeAsk when a model gateway is configured"
  }

  Write-Host ""
  Pass "smoke test"
} finally {
  if ($documentId -and -not $KeepDocument) {
    try {
      Invoke-Json "DELETE" "$apiBase/v1/documents/$documentId`?tenant_id=$TenantId" | Out-Null
      Pass "cleanup" "deleted $documentId"
    } catch {
      Warn "cleanup" $_.Exception.Message
    }
  }

  if (Test-Path -LiteralPath $tempPath) {
    Remove-Item -LiteralPath $tempPath -Force
  }
}
