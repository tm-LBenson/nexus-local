param(
  [switch]$Volumes
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$composeFile = Join-Path $root "deploy\compose\compose.cpu.yml"
$embeddingsComposeFile = Join-Path $root "deploy\compose\compose.embeddings.yml"
$embeddingsGPUComposeFile = Join-Path $root "deploy\compose\compose.embeddings.gpu.yml"
$envFile = Join-Path $root ".env"

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
  throw "docker is required but was not found on PATH"
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
$composeArgs += "down"
if ($Volumes) {
  $composeArgs += "--volumes"
}

Write-Host "Stopping Nexus Local..."
& docker @composeArgs
Write-Host "Stopped."
