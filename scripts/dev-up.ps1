param(
  [switch]$NoBuild,
  [int]$TimeoutSeconds = 180
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$composeFile = Join-Path $root "deploy\compose\compose.cpu.yml"

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

$composeArgs = @("compose", "-f", $composeFile, "up", "-d")
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
Write-Host ""
Write-Host "Run scripts\dev-check.ps1 for a service check."
Write-Host "Run scripts\dev-check.ps1 -Smoke to verify upload, ingestion, and search."
