$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot

$launcherEnv = Join-Path ([System.IO.Path]::GetTempPath()) "nexus-launcher-test.env"
try {
  & (Join-Path $root "nexus.ps1") setup `
    -NonInteractive `
    -Profile gpu-local `
    -ProviderPreset starter `
    -EmbeddingRuntime none `
    -ApiHostPort 18080 `
    -WebHostPort 15173 `
    -PublicUrl http://localhost:15173 `
    -ModelGatewayBaseUrl http://model-gateway:8000/v1 `
    -ModelGatewayPort 18000 `
    -GeneralModelId Qwen/Qwen2.5-7B-Instruct `
    -OutputPath $launcherEnv `
    -Force `
    -SkipPortCheck | Out-Null

  $launcherProfile = Select-String -Path $launcherEnv -Pattern "^DEPLOYMENT_PROFILE=gpu-local$" -Quiet
  if (-not $launcherProfile) {
    throw "Launcher setup regression failed: DEPLOYMENT_PROFILE was not written as gpu-local."
  }
  $launcherGatewayPort = Select-String -Path $launcherEnv -Pattern "^MODEL_GATEWAY_PORT=18000$" -Quiet
  if (-not $launcherGatewayPort) {
    throw "Launcher setup regression failed: MODEL_GATEWAY_PORT was not written as 18000."
  }
  $launcherApiPort = Select-String -Path $launcherEnv -Pattern "^API_HOST_PORT=18080$" -Quiet
  if (-not $launcherApiPort) {
    throw "Launcher setup regression failed: API_HOST_PORT was not written as 18080."
  }
  $launcherWebPort = Select-String -Path $launcherEnv -Pattern "^WEB_HOST_PORT=15173$" -Quiet
  if (-not $launcherWebPort) {
    throw "Launcher setup regression failed: WEB_HOST_PORT was not written as 15173."
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
