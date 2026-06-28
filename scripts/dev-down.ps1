param(
  [switch]$Volumes,
  [string]$Profile = ""
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $root ".env"
$composeHelper = Join-Path (Join-Path $PSScriptRoot "lib") "compose.ps1"
. $composeHelper

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
  throw "docker is required but was not found on PATH"
}

$composeConfig = Get-NexusComposeConfig -Root $root -EnvFile $envFile -Profile $Profile
$composeArgs = $composeConfig.Args
$composeArgs += "down"
if ($Volumes) {
  $composeArgs += "--volumes"
}

Write-Host "Stopping Nexus Local ($($composeConfig.Profile))..."
& docker @composeArgs
if ($LASTEXITCODE -ne 0) {
  throw "docker compose down failed with exit code $LASTEXITCODE. Make sure Docker Desktop is running with the Linux engine started."
}
Write-Host "Stopped."
