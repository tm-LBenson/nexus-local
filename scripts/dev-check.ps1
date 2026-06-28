param(
  [switch]$Smoke,
  [switch]$SkipAsk,
  [switch]$IncludeAsk,
  [string]$ApiUrl = "http://localhost:8080",
  [string]$TenantId = "",
  [string]$WorkspaceName = "",
  [string]$FixturePath = "",
  [string]$ModelTarget = "general",
  [string]$UserId = "",
  [string]$UserEmail = "",
  [int]$SmokeTimeoutSeconds = 90,
  [string]$Profile = ""
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $root ".env"
$smokeScript = Join-Path $PSScriptRoot "dev-smoke.ps1"
$composeHelper = Join-Path (Join-Path $PSScriptRoot "lib") "compose.ps1"
. $composeHelper
$failures = 0

function Pass($name, $detail = "") {
  if ($detail) {
    Write-Host "[OK]   $name - $detail"
  } else {
    Write-Host "[OK]   $name"
  }
}

function Warn($name, $detail = "") {
  if ($detail) {
    Write-Host "[WARN] $name - $detail"
  } else {
    Write-Host "[WARN] $name"
  }
}

function Fail($name, $detail = "") {
  $script:failures++
  if ($detail) {
    Write-Host "[FAIL] $name - $detail"
  } else {
    Write-Host "[FAIL] $name"
  }
}

function Test-Command($name, $required = $true) {
  $cmd = Get-Command $name -ErrorAction SilentlyContinue
  if ($cmd) {
    Pass $name $cmd.Source
    return $true
  }
  if ($required) {
    Fail $name "not found on PATH"
  } else {
    Warn $name "not found on PATH"
  }
  return $false
}

function Test-Http($name, $url, $required = $true) {
  try {
    $response = Invoke-WebRequest -UseBasicParsing -Uri $url -TimeoutSec 5
    Pass $name "$($response.StatusCode) $url"
    return $true
  } catch {
    if ($required) {
      Fail $name $_.Exception.Message
    } else {
      Warn $name $_.Exception.Message
    }
    return $false
  }
}

function Test-Tcp($name, $hostName, $port, $required = $true) {
  try {
    $client = [System.Net.Sockets.TcpClient]::new()
    $connect = $client.BeginConnect($hostName, $port, $null, $null)
    if (-not $connect.AsyncWaitHandle.WaitOne(1500)) {
      throw "timeout"
    }
    $client.EndConnect($connect)
    $client.Close()
    Pass $name "$hostName`:$port"
    return $true
  } catch {
    if ($required) {
      Fail $name "$hostName`:$port is not reachable"
    } else {
      Warn $name "$hostName`:$port is not reachable"
    }
    return $false
  }
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

Write-Host "Nexus Local dev check"
Write-Host ""

$composeConfig = Get-NexusComposeConfig -Root $root -EnvFile $envFile -Profile $Profile
$embeddingRuntime = $composeConfig.EmbeddingRuntime
$apiHostPort = Read-NexusEnvValue $envFile "API_HOST_PORT"
$webHostPort = Read-NexusEnvValue $envFile "WEB_HOST_PORT"
if (-not $apiHostPort) {
  $apiHostPort = "8080"
}
if (-not $webHostPort) {
  $webHostPort = "5173"
}
if ($ApiUrl -eq "http://localhost:8080" -and $apiHostPort -ne "8080") {
  $ApiUrl = "http://localhost:$apiHostPort"
}
Pass "compose profile" "$($composeConfig.Profile) / embeddings: $embeddingRuntime"

$hasDocker = Test-Command docker
Test-Command go $false | Out-Null
Test-Command node $false | Out-Null
Test-Command npm $false | Out-Null

if ($hasDocker) {
  try {
    & docker version | Out-Null
    if ($LASTEXITCODE -ne 0) {
      throw "docker version failed with exit code $LASTEXITCODE"
    }
    Pass "docker daemon"
  } catch {
    Fail "docker daemon" $_.Exception.Message
  }

  try {
    $composeArgs = $composeConfig.Args
    $composeArgs += @("config", "--quiet")
    & docker @composeArgs
    if ($LASTEXITCODE -ne 0) {
      throw "docker compose config failed with exit code $LASTEXITCODE"
    }
    Pass "compose config" ($composeConfig.Files -join ", ")
  } catch {
    Fail "compose config" $_.Exception.Message
  }

  try {
    Write-Host ""
    $composeArgs = $composeConfig.Args
    $composeArgs += "ps"
    & docker @composeArgs
    if ($LASTEXITCODE -ne 0) {
      throw "docker compose ps failed with exit code $LASTEXITCODE"
    }
  } catch {
    Warn "compose ps" $_.Exception.Message
  }
}

Write-Host ""
Test-Http "web" "http://localhost:$webHostPort" $false | Out-Null
Test-Http "api health" "$($ApiUrl.TrimEnd('/'))/healthz" $false | Out-Null
Test-Http "api readiness" "$($ApiUrl.TrimEnd('/'))/readyz" $false | Out-Null
Test-Http "qdrant" "http://localhost:6333" $false | Out-Null
Test-Tcp "postgres" "localhost" 5432 $false | Out-Null
Test-Tcp "minio api" "localhost" 9000 $false | Out-Null
Test-Tcp "minio console" "localhost" 9001 $false | Out-Null
Test-Tcp "nats" "localhost" 4222 $false | Out-Null
Test-Tcp "valkey" "localhost" 6379 $false | Out-Null
if ($embeddingRuntime -in @("cpu", "gpu")) {
  Test-Http "embedding gateway" "http://localhost:8082/docs" $false | Out-Null
}

if ($Smoke) {
  Write-Host ""
  try {
    $smokeParams = @{
      ApiUrl = $ApiUrl
      TimeoutSeconds = $SmokeTimeoutSeconds
    }
    if (-not [string]::IsNullOrWhiteSpace($TenantId)) {
      $smokeParams.TenantId = $TenantId
    }
    if (-not [string]::IsNullOrWhiteSpace($WorkspaceName)) {
      $smokeParams.WorkspaceName = $WorkspaceName
    }
    if (-not [string]::IsNullOrWhiteSpace($FixturePath)) {
      $smokeParams.FixturePath = $FixturePath
    }
    if (-not [string]::IsNullOrWhiteSpace($ModelTarget)) {
      $smokeParams.ModelTarget = $ModelTarget
    }
    if (-not [string]::IsNullOrWhiteSpace($UserId)) {
      $smokeParams.UserId = $UserId
    }
    if (-not [string]::IsNullOrWhiteSpace($UserEmail)) {
      $smokeParams.UserEmail = $UserEmail
    }
    if ($SkipAsk) {
      $smokeParams.SkipAsk = $true
    }
    if ($IncludeAsk) {
      $smokeParams.IncludeAsk = $true
    }
    & $smokeScript @smokeParams
  } catch {
    Fail "smoke test" $_.Exception.Message
  }
}

Write-Host ""
if ($failures -gt 0) {
  throw "$failures required check(s) failed"
}
Pass "required checks"
