param(
  [switch]$Volumes
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$composeFile = Join-Path $root "deploy\compose\compose.cpu.yml"
$envFile = Join-Path $root ".env"

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
  throw "docker is required but was not found on PATH"
}

$composeArgs = @("compose")
if (Test-Path $envFile) {
  $composeArgs += @("--env-file", $envFile)
}
$composeArgs += @("-f", $composeFile, "down")
if ($Volumes) {
  $composeArgs += "--volumes"
}

Write-Host "Stopping Nexus Local..."
& docker @composeArgs
Write-Host "Stopped."
