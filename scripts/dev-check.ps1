$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$composeFile = Join-Path $root "deploy\compose\compose.cpu.yml"
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

Write-Host "Nexus Local dev check"
Write-Host ""

$hasDocker = Test-Command docker
Test-Command go $false | Out-Null
Test-Command node $false | Out-Null
Test-Command npm $false | Out-Null

if ($hasDocker) {
  try {
    & docker version | Out-Null
    Pass "docker daemon"
  } catch {
    Fail "docker daemon" $_.Exception.Message
  }

  try {
    & docker compose -f $composeFile config --quiet
    Pass "compose config" $composeFile
  } catch {
    Fail "compose config" $_.Exception.Message
  }

  try {
    Write-Host ""
    & docker compose -f $composeFile ps
  } catch {
    Warn "compose ps" $_.Exception.Message
  }
}

Write-Host ""
Test-Http "web" "http://localhost:5173" $false | Out-Null
Test-Http "api health" "http://localhost:8080/healthz" $false | Out-Null
Test-Http "api readiness" "http://localhost:8080/readyz" $false | Out-Null
Test-Http "qdrant" "http://localhost:6333" $false | Out-Null
Test-Tcp "postgres" "localhost" 5432 $false | Out-Null
Test-Tcp "minio api" "localhost" 9000 $false | Out-Null
Test-Tcp "minio console" "localhost" 9001 $false | Out-Null
Test-Tcp "nats" "localhost" 4222 $false | Out-Null
Test-Tcp "valkey" "localhost" 6379 $false | Out-Null

Write-Host ""
if ($failures -gt 0) {
  throw "$failures required check(s) failed"
}
Pass "required checks"
