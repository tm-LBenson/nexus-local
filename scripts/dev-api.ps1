$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Push-Location (Join-Path $root "services/api")
try {
  go run ./cmd/api
} finally {
  Pop-Location
}
