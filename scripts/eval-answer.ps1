param(
  [string]$Fixture = "",
  [switch]$Json
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($Fixture)) {
  $Fixture = Join-Path $root "fixtures/eval/answer-baseline.json"
}

if (-not (Test-Path $Fixture)) {
  throw "Answer fixture not found: $Fixture"
}

Push-Location (Join-Path $root "services/api")
try {
  $goArgs = @("run", "./cmd/eval", "-kind", "answer", "-fixture", $Fixture)
  if ($Json) {
    $goArgs += "-json"
  }
  & go @goArgs
  if ($LASTEXITCODE -ne 0) {
    throw "answer eval failed with exit code $LASTEXITCODE"
  }
} finally {
  Pop-Location
}
