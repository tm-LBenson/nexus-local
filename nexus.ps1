$ErrorActionPreference = "Stop"

$launcher = Join-Path $PSScriptRoot "scripts/nexus.ps1"
& $launcher @args
