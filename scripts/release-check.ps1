param(
  [switch]$SkipSmoke,
  [switch]$SkipBackupRestore,
  [switch]$SkipAsk,
  [int]$SmokeTimeoutSeconds = 180,
  [string]$Profile = "",
  [string]$ApiUrl = ""
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$evalAnswerScript = Join-Path $PSScriptRoot "eval-answer.ps1"
$devCheckScript = Join-Path $PSScriptRoot "dev-check.ps1"
$evalRetrievalScript = Join-Path $PSScriptRoot "eval-retrieval.ps1"
$testScript = Join-Path $PSScriptRoot "test.ps1"

function Invoke-ReleaseStep($name, [scriptblock]$block) {
  Write-Host ""
  Write-Host "== $name =="
  & $block
}

function Invoke-WebBuild {
  Push-Location (Join-Path $root "apps/web")
  try {
    & npm run build
    if ($LASTEXITCODE -ne 0) {
      throw "npm run build failed with exit code $LASTEXITCODE"
    }
  } finally {
    Pop-Location
  }
}

Write-Host "Nexus Local release check"
Write-Host "========================="

$skippedGates = [System.Collections.Generic.List[string]]::new()

Invoke-ReleaseStep "API and launcher tests" {
  & $testScript
}

Invoke-ReleaseStep "Retrieval eval gate" {
  & $evalRetrievalScript
}

Invoke-ReleaseStep "Answer eval gate" {
  & $evalAnswerScript
}

Invoke-ReleaseStep "Web production build" {
  Invoke-WebBuild
}

if (-not $SkipSmoke) {
  Invoke-ReleaseStep "End-to-end smoke gate" {
    $smokeArgs = @{
      Smoke = $true
      SmokeTimeoutSeconds = $SmokeTimeoutSeconds
    }
    if ($SkipAsk) {
      $smokeArgs.SkipAsk = $true
    }
    if (-not [string]::IsNullOrWhiteSpace($Profile)) {
      $smokeArgs.Profile = $Profile
    }
    if (-not [string]::IsNullOrWhiteSpace($ApiUrl)) {
      $smokeArgs.ApiUrl = $ApiUrl
    }
    & $devCheckScript @smokeArgs
  }
} else {
  [void]$skippedGates.Add("end-to-end smoke")
  Write-Host ""
  Write-Host "[WARN] Skipped end-to-end smoke gate."
}

if (-not $SkipBackupRestore) {
  Invoke-ReleaseStep "Backup and restore round trip" {
    $backupArgs = @{
      BackupSmoke = $true
      BackupRestoreRoundTrip = $true
      SmokeTimeoutSeconds = $SmokeTimeoutSeconds
    }
    if (-not [string]::IsNullOrWhiteSpace($Profile)) {
      $backupArgs.Profile = $Profile
    }
    if (-not [string]::IsNullOrWhiteSpace($ApiUrl)) {
      $backupArgs.ApiUrl = $ApiUrl
    }
    & $devCheckScript @backupArgs
  }
} else {
  [void]$skippedGates.Add("backup/restore round trip")
  Write-Host ""
  Write-Host "[WARN] Skipped backup and restore round trip."
}

Write-Host ""
Write-Host "Manual UI release checklist"
Write-Host "---------------------------"
Write-Host "[ ] Fresh setup page blocks entry until API/runtime/gateway checks are understandable."
Write-Host "[ ] Create workspace, then run the first-run sample flow through upload, search, and ask."
Write-Host "[ ] Library upload, source scan, retry, delete, download, and detail panels do not jump or hide status."
Write-Host "[ ] Dashboard shows active work, failures, recent work, and no dead controls."
Write-Host "[ ] Activity and Settings filters make audit/job/provider diagnostics findable."
Write-Host "[ ] Slow model answer shows spinner/status, supports cancel, and reports timeout clearly."
Write-Host ""
if ($skippedGates.Count -gt 0) {
  Write-Host "[WARN] Selected release checks completed; skipped: $($skippedGates -join ', ')."
} else {
  Write-Host "[OK] Automated release checks passed."
}
