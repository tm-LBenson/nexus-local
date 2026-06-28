param(
  [string]$Command = "menu",
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$RemainingArgs
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $root ".env"
$webUrl = "http://localhost:5173"

function Write-Header {
  Write-Host ""
  Write-Host "Nexus Local"
  Write-Host "==========="
}

function Read-EnvValue($key) {
  if (-not (Test-Path $envFile)) {
    return ""
  }
  foreach ($line in Get-Content $envFile) {
    if ($line -match "^\s*$([regex]::Escape($key))=(.*)$") {
      return $matches[1].Trim()
    }
  }
  return ""
}

function Show-EnvironmentSummary {
  $profile = Read-EnvValue "DEPLOYMENT_PROFILE"
  $providerPreset = Read-EnvValue "PROVIDER_PRESET"
  $embeddingRuntime = Read-EnvValue "EMBEDDING_RUNTIME"
  $gateway = Read-EnvValue "MODEL_GATEWAY_BASE_URL"
  $model = Read-EnvValue "GENERAL_MODEL_ID"

  if (-not $profile) { $profile = "not configured" }
  if (-not $providerPreset) { $providerPreset = "not configured" }
  if (-not $embeddingRuntime) { $embeddingRuntime = "not configured" }
  if (-not $gateway) { $gateway = "not configured" }
  if (-not $model) { $model = "not configured" }

  Write-Host "Config:  $profile / $providerPreset / embeddings: $embeddingRuntime"
  Write-Host "Gateway: $gateway"
  Write-Host "Model:   $model"
  Write-Host "Web:     $webUrl"
  Write-Host ""
}

function Invoke-LocalScript($scriptName, [string[]]$arguments = @()) {
  $scriptPath = Join-Path $PSScriptRoot $scriptName
  if (-not (Test-Path $scriptPath)) {
    throw "Missing script: $scriptPath"
  }
  & $scriptPath @arguments
}

function Open-WebApp {
  Write-Host "Opening $webUrl"
  Start-Process $webUrl
}

function Confirm-Action($prompt, $expected = "yes") {
  $answer = Read-Host $prompt
  return $answer.Trim().Equals($expected, [StringComparison]::OrdinalIgnoreCase)
}

function Pause-Menu {
  Write-Host ""
  Read-Host "Press Enter to continue" | Out-Null
}

function Show-Help {
  Write-Host @"
Nexus Local launcher

Usage:
  .\nexus.ps1
  .\nexus.ps1 <command> [script arguments]
  ./nexus
  ./nexus <command> [script arguments]

Commands:
  menu             Open the interactive TUI menu
  setup            Run guided environment setup
  up, start        Start the container stack
  down, stop       Stop the container stack
  restart          Stop, then start the container stack
  reset-volumes    Stop and remove local Docker volumes
  status, check    Run environment and service checks
  smoke            Run the end-to-end smoke test
  smoke-no-ask     Run smoke test without the model gateway Ask step
  open, ui         Open the web UI
  backup           Create a backup
  restore          Restore from a backup path
  test             Run API tests
  help             Show this help

Examples:
  .\nexus.ps1
  .\nexus.ps1 setup -Profile cpu-lite -ProviderPreset starter -Force
  .\nexus.ps1 up
  .\nexus.ps1 smoke-no-ask
  .\nexus.ps1 backup -Name before-upgrade
  ./nexus up
  ./nexus smoke-no-ask
"@
}

function Invoke-CommandMode($name, [string[]]$arguments = @()) {
  $normalized = $name.Trim().ToLowerInvariant()
  if (-not $normalized) {
    $normalized = "menu"
  }

  switch ($normalized) {
    "menu" {
      Show-Menu
    }
    "help" {
      Show-Help
    }
    "setup" {
      Invoke-LocalScript "setup.ps1" $arguments
    }
    "configure" {
      Invoke-LocalScript "setup.ps1" $arguments
    }
    "up" {
      Invoke-LocalScript "dev-up.ps1" $arguments
    }
    "start" {
      Invoke-LocalScript "dev-up.ps1" $arguments
    }
    "down" {
      Invoke-LocalScript "dev-down.ps1" $arguments
    }
    "stop" {
      Invoke-LocalScript "dev-down.ps1" $arguments
    }
    "restart" {
      Invoke-LocalScript "dev-down.ps1"
      Invoke-LocalScript "dev-up.ps1" $arguments
    }
    "reset-volumes" {
      Invoke-LocalScript "dev-down.ps1" ($arguments + @("-Volumes"))
    }
    "status" {
      Invoke-LocalScript "dev-check.ps1" $arguments
    }
    "check" {
      Invoke-LocalScript "dev-check.ps1" $arguments
    }
    "smoke" {
      Invoke-LocalScript "dev-check.ps1" (@("-Smoke") + $arguments)
    }
    "smoke-no-ask" {
      Invoke-LocalScript "dev-check.ps1" (@("-Smoke", "-SkipAsk") + $arguments)
    }
    "smoke-lite" {
      Invoke-LocalScript "dev-check.ps1" (@("-Smoke", "-SkipAsk") + $arguments)
    }
    "open" {
      Open-WebApp
    }
    "ui" {
      Open-WebApp
    }
    "backup" {
      Invoke-LocalScript "backup.ps1" $arguments
    }
    "restore" {
      Invoke-LocalScript "restore.ps1" $arguments
    }
    "test" {
      Invoke-LocalScript "test.ps1" $arguments
    }
    default {
      Write-Host "Unknown command: $name"
      Write-Host ""
      Show-Help
      exit 1
    }
  }
}

function Invoke-RestoreFromMenu {
  $backupPath = Read-Host "Backup path"
  if ([string]::IsNullOrWhiteSpace($backupPath)) {
    Write-Host "Restore cancelled."
    return
  }
  if (-not (Confirm-Action "Restore will overwrite local data. Type RESTORE to continue" "RESTORE")) {
    Write-Host "Restore cancelled."
    return
  }
  Invoke-LocalScript "restore.ps1" @("-BackupPath", $backupPath)
}

function Show-Menu {
  while ($true) {
    Write-Header
    Show-EnvironmentSummary
    Write-Host "1. Guided setup / configure"
    Write-Host "2. Start stack"
    Write-Host "3. Open web UI"
    Write-Host "4. Check status"
    Write-Host "5. Smoke test"
    Write-Host "6. Smoke test without Ask"
    Write-Host "7. Backup data"
    Write-Host "8. Restore backup"
    Write-Host "9. Stop stack"
    Write-Host "10. Stop stack and remove volumes"
    Write-Host "11. Run API tests"
    Write-Host "H. Help"
    Write-Host "Q. Quit"
    Write-Host ""

    $choice = (Read-Host "Select").Trim().ToLowerInvariant()
    try {
      switch ($choice) {
        "1" { Invoke-LocalScript "setup.ps1"; Pause-Menu }
        "2" { Invoke-LocalScript "dev-up.ps1"; Pause-Menu }
        "3" { Open-WebApp; Pause-Menu }
        "4" { Invoke-LocalScript "dev-check.ps1"; Pause-Menu }
        "5" { Invoke-LocalScript "dev-check.ps1" @("-Smoke"); Pause-Menu }
        "6" { Invoke-LocalScript "dev-check.ps1" @("-Smoke", "-SkipAsk"); Pause-Menu }
        "7" { Invoke-LocalScript "backup.ps1"; Pause-Menu }
        "8" { Invoke-RestoreFromMenu; Pause-Menu }
        "9" { Invoke-LocalScript "dev-down.ps1"; Pause-Menu }
        "10" {
          if (Confirm-Action "This removes local database, object, queue, cache, and vector volumes. Type REMOVE to continue" "REMOVE") {
            Invoke-LocalScript "dev-down.ps1" @("-Volumes")
          } else {
            Write-Host "Volume removal cancelled."
          }
          Pause-Menu
        }
        "11" { Invoke-LocalScript "test.ps1"; Pause-Menu }
        "h" { Show-Help; Pause-Menu }
        "help" { Show-Help; Pause-Menu }
        "q" { return }
        "quit" { return }
        "exit" { return }
        default {
          Write-Host "Choose a menu item, H for help, or Q to quit."
          Pause-Menu
        }
      }
    } catch {
      Write-Host ""
      Write-Host "[ERROR] $($_.Exception.Message)"
      Pause-Menu
    }
  }
}

Invoke-CommandMode $Command $RemainingArgs
