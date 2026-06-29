param(
  [switch]$NoBuild,
  [int]$TimeoutSeconds = 180,
  [string]$Profile = ""
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

Require-Command docker

$composeConfig = Get-NexusComposeConfig -Root $root -EnvFile $envFile -Profile $Profile
$embeddingRuntime = $composeConfig.EmbeddingRuntime
$apiHostPort = Read-NexusEnvValue $envFile "API_HOST_PORT"
$webHostPort = Read-NexusEnvValue $envFile "WEB_HOST_PORT"
$sourceHostPath = Read-NexusEnvValue $envFile "NEXUS_SOURCE_HOST_PATH"
$sourceContainerPath = Read-NexusEnvValue $envFile "NEXUS_SOURCE_CONTAINER_PATH"
if (-not $apiHostPort) {
  $apiHostPort = "8080"
}
if (-not $webHostPort) {
  $webHostPort = "5173"
}
$composeArgs = $composeConfig.Args
$composeArgs += @("up", "-d")
if (-not $NoBuild) {
  $composeArgs += "--build"
}

Write-Host "Starting Nexus Local ($($composeConfig.Profile))..."
& docker @composeArgs
if ($LASTEXITCODE -ne 0) {
  throw "docker compose up failed with exit code $LASTEXITCODE. Make sure Docker Desktop is running with the Linux engine started."
}

Wait-Http "API health" "http://localhost:$apiHostPort/healthz" $TimeoutSeconds
Wait-Http "API readiness" "http://localhost:$apiHostPort/readyz" $TimeoutSeconds
Wait-Http "Web" "http://localhost:$webHostPort" $TimeoutSeconds

Write-Host ""
Write-Host "Nexus Local is running"
Write-Host "Web:          http://localhost:$webHostPort"
Write-Host "API:          http://localhost:$apiHostPort"
Write-Host "MinIO:        http://localhost:9001"
Write-Host "Qdrant:       http://localhost:6333"
if ($composeConfig.Profile -eq "gpu-local") {
  $modelGatewayPort = Read-NexusEnvValue $envFile "MODEL_GATEWAY_PORT"
  if (-not $modelGatewayPort) {
    $modelGatewayPort = "8000"
  }
  Write-Host "Model gateway: http://localhost:$modelGatewayPort/v1"
}
if ($embeddingRuntime -in @("cpu", "gpu")) {
  Write-Host "Embeddings:  http://localhost:8082"
}
if ($composeConfig.SourceMounts) {
  if (-not $sourceContainerPath) {
    $sourceContainerPath = "/sources/primary"
  }
  Write-Host "Source:      $sourceHostPath -> $sourceContainerPath"
}
Write-Host ""
Write-Host "Run scripts\dev-check.ps1 for a service check."
Write-Host "Run scripts\dev-check.ps1 -Smoke for the full E2E release gate."
Write-Host "Run scripts\dev-check.ps1 -Smoke -SkipAsk when no model gateway is running."
