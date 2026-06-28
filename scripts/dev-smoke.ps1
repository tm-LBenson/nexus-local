param(
  [string]$ApiUrl = "http://localhost:8080",
  [string]$TenantId = "",
  [string]$WorkspaceName = "",
  [string]$FixturePath = "",
  [string]$ModelTarget = "general",
  [string]$UserId = "",
  [string]$UserEmail = "",
  [int]$TimeoutSeconds = 120,
  [switch]$SkipAsk,
  [switch]$IncludeAsk,
  [switch]$KeepDocument
)

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Net.Http

$root = Split-Path -Parent $PSScriptRoot
$apiBase = $ApiUrl.TrimEnd("/")
$runId = [Guid]::NewGuid().ToString("N").Substring(0, 12)
$workspaceCreated = $false
$workspaceID = ""
$documentName = "nexus-local-smoke-$runId.md"
$documentId = $null
$conversationId = $null
$question = "What is the Nexus Local smoke phrase for this run?"
$needle = "nexus-local-smoke-needle-$runId"
$tempPath = Join-Path ([System.IO.Path]::GetTempPath()) $documentName

if ([string]::IsNullOrWhiteSpace($FixturePath)) {
  $FixturePath = Join-Path $root "fixtures/smoke/nexus-smoke.md"
}

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

function New-SmokeDocument($fixturePath, $outputPath, $runId, $needle) {
  if (-not (Test-Path -LiteralPath $fixturePath)) {
    throw "smoke fixture was not found: $fixturePath"
  }

  $content = Get-Content -LiteralPath $fixturePath -Raw
  $content = $content.Replace("{{SMOKE_RUN_ID}}", $runId)
  $content = $content.Replace("{{SMOKE_NEEDLE}}", $needle)
  if ($content -notlike "*$needle*") {
    $content = "$content`n`nUnique smoke phrase: $needle`n"
  }
  Set-Content -LiteralPath $outputPath -Encoding UTF8 -Value $content
}

function Wait-DocumentReady($tenantId, $documentId) {
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

function Assert-HasHit($hits, $documentId, $needle, $name) {
  $hitList = @($hits)
  if ($hitList.Count -lt 1) {
    throw "$name returned no hits"
  }

  $documentHits = @($hitList | Where-Object { $_.document_id -eq $documentId })
  if ($documentHits.Count -lt 1) {
    throw "$name returned hits, but none referenced $documentId"
  }

  $needleHits = @($documentHits | Where-Object { $_.text -like "*$needle*" })
  if ($needleHits.Count -lt 1) {
    throw "$name returned $documentId, but no hit contained the smoke phrase"
  }

  return $hitList
}

try {
  Write-Host "Nexus Local end-to-end smoke test"
  Write-Host ""

  if ($IncludeAsk -and $SkipAsk) {
    Warn "ask flags" "-SkipAsk overrides -IncludeAsk"
  }

  $health = Invoke-Json "GET" "$apiBase/healthz"
  Pass "api health" "$($health.status) $($health.version)"

  $readiness = Invoke-Json "GET" "$apiBase/readyz"
  Pass "api readiness" "persistence=$($readiness.persistence_backend), objects=$($readiness.object_storage_backend), vectors=$($readiness.vector_backend), embeddings=$($readiness.embedding_backend)"

  $me = Invoke-Json "GET" "$apiBase/v1/me"
  Pass "current user" "$($me.user.email)"

  if ([string]::IsNullOrWhiteSpace($TenantId)) {
    if ([string]::IsNullOrWhiteSpace($WorkspaceName)) {
      $WorkspaceName = "Nexus Smoke $runId"
    }
    $workspace = Invoke-Json "POST" "$apiBase/v1/tenants" @{
      name = $WorkspaceName
    }
    $TenantId = $workspace.tenant.id
    $workspaceID = $TenantId
    $workspaceCreated = $true
    Pass "workspace" "$WorkspaceName -> $TenantId"
  } else {
    $workspaceID = $TenantId
    Pass "workspace" "using existing $TenantId"
  }

  New-SmokeDocument $FixturePath $tempPath $runId $needle
  Pass "fixture" ([System.IO.Path]::GetFullPath($FixturePath))

  $upload = Invoke-DocumentUpload "$apiBase/v1/documents/upload" $TenantId $tempPath
  $documentId = $upload.document.id
  Pass "upload" "$documentName -> $documentId"

  $detail = Wait-DocumentReady $TenantId $documentId
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
  $searchHits = Assert-HasHit $search.hits $documentId $needle "search"
  Pass "search" "$($searchHits.Count) hit(s), first score=$($searchHits[0].score)"

  if ($SkipAsk) {
    Warn "ask" "skipped; run without -SkipAsk when a model gateway is configured"
    Warn "history" "skipped because ask was skipped"
  } else {
    $ask = Invoke-Json "POST" "$apiBase/v1/conversations/ask" @{
      tenant_id = $TenantId
      document_id = $documentId
      model_target = $ModelTarget
      question = $question
      limit = 5
    }
    $conversationId = $ask.conversation.id
    if ([string]::IsNullOrWhiteSpace($ask.assistant_message.content)) {
      throw "ask completed but returned no assistant message content"
    }
    $askHits = Assert-HasHit $ask.hits $documentId $needle "ask"
    Pass "ask" "conversation=$conversationId, hits=$($askHits.Count)"

    $encodedTenant = UrlEncode $TenantId
    $history = Invoke-Json "GET" "$apiBase/v1/conversations?tenant_id=$encodedTenant&limit=10"
    $conversationMatches = @($history.conversations | Where-Object { $_.id -eq $conversationId })
    if ($conversationMatches.Count -lt 1) {
      throw "history did not list conversation $conversationId"
    }

    $encodedConversation = UrlEncode $conversationId
    $messages = Invoke-Json "GET" "$apiBase/v1/conversations/$encodedConversation/messages?tenant_id=$encodedTenant"
    $messageList = @($messages.messages)
    if ($messageList.Count -lt 2) {
      throw "history returned $($messageList.Count) messages for $conversationId, want at least 2"
    }
    $userMessages = @($messageList | Where-Object { $_.role -eq "user" -and $_.content -eq $question })
    $assistantMessages = @($messageList | Where-Object { $_.role -eq "assistant" -and -not [string]::IsNullOrWhiteSpace($_.content) })
    if ($userMessages.Count -lt 1 -or $assistantMessages.Count -lt 1) {
      throw "history did not contain the expected user and assistant messages"
    }
    Pass "history" "$($messageList.Count) message(s)"
  }

  $conversationSummary = $conversationId
  if ([string]::IsNullOrWhiteSpace($conversationSummary)) {
    $conversationSummary = "skipped"
  }
  Write-Host ""
  Pass "smoke test" "workspace=$workspaceID document=$documentId conversation=$conversationSummary"
} finally {
  if ($documentId -and -not $KeepDocument) {
    try {
      $encodedTenant = UrlEncode $TenantId
      $encodedDocument = UrlEncode $documentId
      Invoke-Json "DELETE" "$apiBase/v1/documents/$encodedDocument`?tenant_id=$encodedTenant" | Out-Null
      Pass "cleanup document" "deleted $documentId"
    } catch {
      Warn "cleanup document" $_.Exception.Message
    }
  }

  if ($workspaceCreated) {
    Warn "cleanup workspace" "left $workspaceID; workspace deletion is not implemented yet"
  }

  if (Test-Path -LiteralPath $tempPath) {
    Remove-Item -LiteralPath $tempPath -Force
  }
}
