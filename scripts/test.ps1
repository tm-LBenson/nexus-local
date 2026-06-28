$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot

$launcherEnv = Join-Path ([System.IO.Path]::GetTempPath()) "nexus-launcher-test.env"
try {
  & (Join-Path $root "nexus.ps1") setup `
    -NonInteractive `
    -Profile gpu-local `
    -ProviderPreset starter `
    -EmbeddingRuntime none `
    -PublicUrl http://localhost:5173 `
    -ModelGatewayBaseUrl http://model-gateway:8000/v1 `
    -GeneralModelId Qwen/Qwen2.5-7B-Instruct `
    -OutputPath $launcherEnv `
    -Force `
    -SkipPortCheck | Out-Null

  $launcherProfile = Select-String -Path $launcherEnv -Pattern "^DEPLOYMENT_PROFILE=gpu-local$" -Quiet
  if (-not $launcherProfile) {
    throw "Launcher setup regression failed: DEPLOYMENT_PROFILE was not written as gpu-local."
  }
} finally {
  if (Test-Path $launcherEnv) {
    Remove-Item -LiteralPath $launcherEnv -Force
  }
}

Push-Location (Join-Path $root "services/api")
try {
  go test ./...
} finally {
  Pop-Location
}
