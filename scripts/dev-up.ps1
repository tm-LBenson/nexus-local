param(
  [switch]$NoBuild,
  [int]$TimeoutSeconds = 180
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$composeFile = Join-Path $root "deploy\compose\compose.cpu.yml"
$embeddingsComposeFile = Join-Path $root "deploy\compose\compose.embeddings.yml"
$embeddingsGPUComposeFile = Join-Path $root "deploy\compose\compose.embeddings.gpu.yml"
$envFile = Join-Path $root ".env"

function Require-Command($name) {
  if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
    throw "$name is required but was not found on PATH"
  }
}

function Wait-Http($name, $url, $timeoutSeconds) {
  $deadline = (Get-Date).AddSeconds($timeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    try {
      $response = Invoke-WebRequest -UseBasicParsing -Uri $url -TimeoutSec 5
      if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500) {
        Write-Host "$name ready: $url"
        return
      }
    } catch {
      Start-Sleep -Seconds 2
    }
  }
  throw "$name did not become ready before timeout: $url"
}

function Read-EnvValue($path, $key) {
  if (-not (Test-Path $path)) {
    return ""
  }
  foreach ($line in Get-Content $path) {
    if ($line -match "^\s*$([regex]::Escape($key))=(.*)$") {
      return $matches[1].Trim()
    }
  }
  return ""
}

Require-Command docker

$embeddingRuntime = (Read-EnvValue $envFile "EMBEDDING_RUNTIME").ToLowerInvariant()

$composeArgs = @("compose")
if (Test-Path $envFile) {
  $composeArgs += @("--env-file", $envFile)
}
$composeArgs += @("-f", $composeFile)
if ($embeddingRuntime -in @("cpu", "gpu")) {
  $composeArgs += @("-f", $embeddingsComposeFile)
}
if ($embeddingRuntime -eq "gpu") {
  $composeArgs += @("-f", $embeddingsGPUComposeFile)
}
$composeArgs += @("up", "-d")
if (-not $NoBuild) {
  $composeArgs += "--build"
}

Write-Host "Starting Nexus Local..."
& docker @composeArgs

Wait-Http "API health" "http://localhost:8080/healthz" $TimeoutSeconds
Wait-Http "API readiness" "http://localhost:8080/readyz" $TimeoutSeconds
Wait-Http "Web" "http://localhost:5173" $TimeoutSeconds

Write-Host ""
Write-Host "Nexus Local is running"
Write-Host "Web:          http://localhost:5173"
Write-Host "API:          http://localhost:8080"
Write-Host "MinIO:        http://localhost:9001"
Write-Host "Qdrant:       http://localhost:6333"
if ($embeddingRuntime -in @("cpu", "gpu")) {
  Write-Host "Embeddings:  http://localhost:8082"
}
Write-Host ""
Write-Host "Run scripts\dev-check.ps1 for a service check."
Write-Host "Run scripts\dev-check.ps1 -Smoke for the full E2E release gate."
Write-Host "Run scripts\dev-check.ps1 -Smoke -SkipAsk when no model gateway is running."
