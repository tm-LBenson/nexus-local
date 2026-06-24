$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Push-Location "$root\services\api"
try {
  go test ./...
} finally {
  Pop-Location
}

