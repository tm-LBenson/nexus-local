param(
  [string]$Command = "launch",
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
  if (-not (Test-Path $envFile)) {
    Write-Host "Config:  no .env yet"
    Write-Host "Web:     $webUrl"
    Write-Host ""
    return
  }

  $profile = Read-EnvValue "DEPLOYMENT_PROFILE"
  $providerPreset = Read-EnvValue "PROVIDER_PRESET"
  $embeddingRuntime = Read-EnvValue "EMBEDDING_RUNTIME"
  $gateway = Read-EnvValue "MODEL_GATEWAY_BASE_URL"
  $model = Read-EnvValue "GENERAL_MODEL_ID"

  if (-not $profile) { $profile = "cpu-lite (default)" }
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

function Convert-ArgumentTokens($arguments) {
  $named = @{}
  $positionals = [System.Collections.Generic.List[string]]::new()
  for ($i = 0; $i -lt $arguments.Count; $i++) {
    $token = [string]$arguments[$i]
    if ($token.StartsWith("-") -and $token.Length -gt 1) {
      $name = $token.TrimStart("-")
      $value = $true
      if ($name.Contains("=")) {
        $parts = $name.Split("=", 2)
        $name = $parts[0]
        $value = $parts[1]
      } elseif (($i + 1) -lt $arguments.Count) {
        $next = [string]$arguments[$i + 1]
        if (-not ($next.StartsWith("-") -and $next.Length -gt 1)) {
          $value = $next
          $i++
        }
      }
      $named[$name] = $value
    } else {
      [void]$positionals.Add($token)
    }
  }

  return [pscustomobject]@{
    Named = $named
    Positionals = [string[]]$positionals.ToArray()
  }
}

function Invoke-LocalScript($scriptName, [string[]]$arguments = @()) {
  $scriptPath = Join-Path $PSScriptRoot $scriptName
  if (-not (Test-Path $scriptPath)) {
    throw "Missing script: $scriptPath"
  }
  $splat = Convert-ArgumentTokens $arguments
  $namedArgs = $splat.Named
  $positionalArgs = $splat.Positionals
  if ($positionalArgs.Count -gt 0) {
    & $scriptPath @positionalArgs @namedArgs
  } else {
    & $scriptPath @namedArgs
  }
}

function Open-WebApp {
  Write-Host "Opening $webUrl"
  if ($IsWindows -or $PSVersionTable.PSEdition -eq "Desktop") {
    Start-Process $webUrl
    return
  }
  if (Get-Command xdg-open -ErrorAction SilentlyContinue) {
    & xdg-open $webUrl | Out-Null
    return
  }
  if (Get-Command open -ErrorAction SilentlyContinue) {
    & open $webUrl | Out-Null
    return
  }
  Write-Host "Open this URL in your browser: $webUrl"
}

function Confirm-Action($prompt, $expected = "yes") {
  $answer = Read-Host $prompt
  return $answer.Trim().Equals($expected, [StringComparison]::OrdinalIgnoreCase)
}

function Read-YesNo($prompt, $defaultYes = $true) {
  $suffix = "[Y/n]"
  if (-not $defaultYes) {
    $suffix = "[y/N]"
  }
  while ($true) {
    $answer = (Read-Host "$prompt $suffix").Trim().ToLowerInvariant()
    if ([string]::IsNullOrWhiteSpace($answer)) {
      return $defaultYes
    }
    if ($answer -in @("y", "yes")) {
      return $true
    }
    if ($answer -in @("n", "no")) {
      return $false
    }
    Write-Host "Use yes or no."
  }
}

function Read-DefaultValue($prompt, $defaultValue) {
  $answer = Read-Host "$prompt [$defaultValue]"
  if ([string]::IsNullOrWhiteSpace($answer)) {
    return $defaultValue
  }
  return $answer.Trim()
}

function Test-PortOpen($port) {
  try {
    $client = [System.Net.Sockets.TcpClient]::new()
    $async = $client.BeginConnect("127.0.0.1", $port, $null, $null)
    if (-not $async.AsyncWaitHandle.WaitOne(300)) {
      $client.Close()
      return $false
    }
    $client.EndConnect($async)
    $client.Close()
    return $true
  } catch {
    return $false
  }
}

function Find-AvailablePort($startPort) {
  for ($port = $startPort; $port -lt ($startPort + 100); $port++) {
    if (-not (Test-PortOpen $port)) {
      return $port.ToString()
    }
  }
  return $startPort.ToString()
}

function Select-Option($title, $options, $defaultValue) {
  Write-Host ""
  Write-Host $title
  for ($i = 0; $i -lt $options.Count; $i++) {
    $option = $options[$i]
    $marker = " "
    if ($option.Value -eq $defaultValue) {
      $marker = "*"
    }
    Write-Host ("  {0}. {1} {2}" -f ($i + 1), $marker, $option.Label)
    if ($option.Detail) {
      Write-Host ("     {0}" -f $option.Detail)
    }
  }

  while ($true) {
    $choice = (Read-Host "Select").Trim().ToLowerInvariant()
    if ([string]::IsNullOrWhiteSpace($choice)) {
      $match = @($options | Where-Object { $_.Value -eq $defaultValue } | Select-Object -First 1)
      if ($match.Count -gt 0) {
        return $match[0].Value
      }
      return $options[0].Value
    }
    if ($choice -match '^\d+$') {
      $index = [int]$choice - 1
      if ($index -ge 0 -and $index -lt $options.Count) {
        return $options[$index].Value
      }
    }
    $match = @($options | Where-Object { $_.Value -eq $choice -or $_.Label.ToLowerInvariant() -eq $choice } | Select-Object -First 1)
    if ($match.Count -gt 0) {
      return $match[0].Value
    }
    Write-Host "Choose one of the listed options."
  }
}

function Get-DefaultModelGateway($profile) {
  switch ($profile) {
    "split-nas-gpu" { return "http://desktop-gpu.local:8000/v1" }
    "gpu-local" { return "http://model-gateway:8000/v1" }
    default { return "http://host.docker.internal:8000/v1" }
  }
}

function Get-DefaultModelGatewayPort {
  if (Test-PortOpen 8000) {
    return Find-AvailablePort 8001
  }
  return "8000"
}

function Get-DefaultEmbeddingGateway($profile, $runtime) {
  if ($runtime -in @("cpu", "gpu")) {
    return "http://embedding-gateway:80/v1"
  }
  switch ($profile) {
    "split-nas-gpu" { return "http://desktop-gpu.local:8082/v1" }
    "gpu-local" { return "http://model-gateway:8000/v1" }
    default { return "http://host.docker.internal:8082/v1" }
  }
}

function Get-DefaultPublicUrl($profile) {
  if ($profile -eq "prod-auth") {
    return "http://localhost:8088"
  }
  return "http://localhost:5173"
}

function Add-SetupArgument($arguments, $name, $value) {
  if (-not [string]::IsNullOrWhiteSpace($value)) {
    [void]$arguments.Add($name)
    [void]$arguments.Add($value)
  }
}

function Invoke-GuidedLaunch($startDefault = $true) {
  Write-Header
  Write-Host "Guided launch"
  Write-Host "Pick the deployment shape. The launcher writes .env, then can start the stack."

  if (Test-Path $envFile) {
    Write-Host ""
    Show-EnvironmentSummary
    $existingProfile = Read-EnvValue "DEPLOYMENT_PROFILE"
    $existingGatewayPort = Read-EnvValue "MODEL_GATEWAY_PORT"
    if (-not $existingGatewayPort) {
      $existingGatewayPort = "8000"
    }
    $parsedGatewayPort = 0
    $existingDefault = "use"
    $useDetail = "Start or manage the current .env."
    if ($existingProfile -eq "gpu-local" -and -not [int]::TryParse($existingGatewayPort, [ref]$parsedGatewayPort)) {
      Write-Host "[WARN] MODEL_GATEWAY_PORT is not a valid number: $existingGatewayPort"
      $existingDefault = "reconfigure"
      $useDetail = "Reconfigure to write a valid model gateway host port."
    } elseif ($existingProfile -eq "gpu-local" -and (Test-PortOpen $parsedGatewayPort)) {
      Write-Host "[WARN] Model gateway host port $existingGatewayPort is already in use."
      Write-Host "       Reconfigure and choose a free host port unless Nexus Local already owns it."
      $existingDefault = "reconfigure"
      $useDetail = "May fail if port $existingGatewayPort belongs to another app."
    }
    $envChoice = Select-Option "Existing configuration found" @(
      [pscustomobject]@{ Label = "Use existing config"; Value = "use"; Detail = $useDetail },
      [pscustomobject]@{ Label = "Reconfigure"; Value = "reconfigure"; Detail = "Choose options and overwrite .env." },
      [pscustomobject]@{ Label = "Back"; Value = "back"; Detail = "" }
    ) $existingDefault
    if ($envChoice -eq "back") {
      return
    }
    if ($envChoice -eq "use") {
      if (Read-YesNo "Start the stack now?" $startDefault) {
        Invoke-LocalScript "dev-up.ps1"
        if (Read-YesNo "Open the web UI?" $true) {
          Open-WebApp
        }
        if (Read-YesNo "Run a smoke test without Ask?" $false) {
          Invoke-LocalScript "dev-check.ps1" @("-Smoke", "-SkipAsk")
        }
      }
      return
    }
  }

  $profile = Select-Option "Deployment target" @(
    [pscustomobject]@{ Label = "Local CPU / Docker"; Value = "cpu-lite"; Detail = "Best first run. App, DB, storage, and search in Docker." },
    [pscustomobject]@{ Label = "Split NAS + GPU box"; Value = "split-nas-gpu"; Detail = "App/storage here, model gateway on another machine." },
    [pscustomobject]@{ Label = "Local NVIDIA GPU"; Value = "gpu-local"; Detail = "Starts the app plus local GPU model services." },
    [pscustomobject]@{ Label = "Production auth"; Value = "prod-auth"; Detail = "Reverse proxy/auth profile for a server deployment." }
  ) "cpu-lite"

  $providerDefault = "starter"
  if ($profile -eq "gpu-local") {
    $providerDefault = "semantic"
  }
  $providerPreset = Select-Option "Search quality" @(
    [pscustomobject]@{ Label = "Starter"; Value = "starter"; Detail = "Fastest path. Hash embeddings; good for plumbing tests." },
    [pscustomobject]@{ Label = "Semantic"; Value = "semantic"; Detail = "Real embeddings for useful search and retrieval." }
  ) $providerDefault

  $embeddingRuntime = "none"
  if ($providerPreset -eq "semantic") {
    $runtimeDefault = "cpu"
    if ($profile -eq "gpu-local") {
      $runtimeDefault = "gpu"
    } elseif ($profile -in @("split-nas-gpu", "prod-auth")) {
      $runtimeDefault = "external"
    }
    $embeddingRuntime = Select-Option "Embedding runtime" @(
      [pscustomobject]@{ Label = "External endpoint"; Value = "external"; Detail = "Use an existing OpenAI-compatible embedding service." },
      [pscustomobject]@{ Label = "CPU container"; Value = "cpu"; Detail = "Start local TEI embeddings on CPU." },
      [pscustomobject]@{ Label = "GPU container"; Value = "gpu"; Detail = "Start local TEI embeddings with NVIDIA GPU." }
    ) $runtimeDefault
  } else {
    Write-Host ""
    Write-Host "Embedding runtime: none"
  }

  $publicUrl = Read-DefaultValue "Public URL" (Get-DefaultPublicUrl $profile)
  $modelGateway = Get-DefaultModelGateway $profile
  $modelGatewayPort = ""
  if ($profile -eq "gpu-local") {
    Write-Host "Model gateway URL: $modelGateway (Docker internal)"
    $modelGatewayPort = Read-DefaultValue "Model gateway host port" (Get-DefaultModelGatewayPort)
  } else {
    $modelGateway = Read-DefaultValue "Model gateway URL" $modelGateway
  }
  $modelID = Read-DefaultValue "Model ID" "Qwen/Qwen2.5-7B-Instruct"
  $embeddingGateway = ""
  if ($providerPreset -eq "semantic") {
    $embeddingGateway = Read-DefaultValue "Embedding URL" (Get-DefaultEmbeddingGateway $profile $embeddingRuntime)
  }

  $setupArgs = [System.Collections.Generic.List[string]]::new()
  [void]$setupArgs.Add("-NonInteractive")
  [void]$setupArgs.Add("-Profile")
  [void]$setupArgs.Add($profile)
  [void]$setupArgs.Add("-ProviderPreset")
  [void]$setupArgs.Add($providerPreset)
  [void]$setupArgs.Add("-EmbeddingRuntime")
  [void]$setupArgs.Add($embeddingRuntime)
  Add-SetupArgument $setupArgs "-PublicUrl" $publicUrl
  Add-SetupArgument $setupArgs "-ModelGatewayBaseUrl" $modelGateway
  Add-SetupArgument $setupArgs "-ModelGatewayPort" $modelGatewayPort
  Add-SetupArgument $setupArgs "-GeneralModelId" $modelID
  Add-SetupArgument $setupArgs "-EmbeddingBaseUrl" $embeddingGateway

  if (Test-Path $envFile) {
    if (-not (Read-YesNo "Overwrite the current .env?" $true)) {
      Write-Host "Setup cancelled."
      return
    }
    [void]$setupArgs.Add("-Force")
  }

  Invoke-LocalScript "setup.ps1" $setupArgs.ToArray()

  if (Read-YesNo "Start the stack now?" $startDefault) {
    Invoke-LocalScript "dev-up.ps1"
    if (Read-YesNo "Open the web UI?" $true) {
      Open-WebApp
    }
    if (Read-YesNo "Run a smoke test without Ask?" $false) {
      Invoke-LocalScript "dev-check.ps1" @("-Smoke", "-SkipAsk")
    }
  }
}

function Pause-Menu {
  Write-Host ""
  Read-Host "Press Enter to continue" | Out-Null
}

function Show-Help {
  Write-Host @"
Nexus Local launcher

Usage:
  .\nexus.ps1          Start guided launch
  .\nexus.ps1 <command> [script arguments]
  ./nexus             Start guided launch
  ./nexus <command> [script arguments]

Commands:
  menu             Open the interactive TUI menu
  setup            Open the guided setup wizard
  up, start        Start the container stack
  run, launch      Open the guided setup-and-start wizard
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
  ./nexus
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
      if ($arguments.Count -gt 0) {
        Invoke-LocalScript "setup.ps1" $arguments
      } else {
        Invoke-GuidedLaunch $false
      }
    }
    "configure" {
      if ($arguments.Count -gt 0) {
        Invoke-LocalScript "setup.ps1" $arguments
      } else {
        Invoke-GuidedLaunch $false
      }
    }
    "up" {
      Invoke-LocalScript "dev-up.ps1" $arguments
    }
    "start" {
      Invoke-LocalScript "dev-up.ps1" $arguments
    }
    "run" {
      if ($arguments.Count -gt 0) {
        Invoke-LocalScript "setup.ps1" $arguments
        Invoke-LocalScript "dev-up.ps1"
      } else {
        Invoke-GuidedLaunch $true
      }
    }
    "launch" {
      if ($arguments.Count -gt 0) {
        Invoke-LocalScript "setup.ps1" $arguments
        Invoke-LocalScript "dev-up.ps1"
      } else {
        Invoke-GuidedLaunch $true
      }
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
    Write-Host "1. Guided launch"
    Write-Host "2. Configure only"
    Write-Host "3. Start"
    Write-Host "4. Open UI"
    Write-Host "5. Check"
    Write-Host "6. Smoke test"
    Write-Host "7. Smoke test without Ask"
    Write-Host "8. Backup"
    Write-Host "9. Restore"
    Write-Host "10. Stop"
    Write-Host "11. Reset volumes"
    Write-Host "12. API tests"
    Write-Host "H. Help"
    Write-Host "Q. Quit"
    Write-Host ""

    $choice = (Read-Host "Select").Trim().ToLowerInvariant()
    try {
      switch ($choice) {
        "1" { Invoke-GuidedLaunch $true; Pause-Menu }
        "2" { Invoke-GuidedLaunch $false; Pause-Menu }
        "3" { Invoke-LocalScript "dev-up.ps1"; Pause-Menu }
        "4" { Open-WebApp; Pause-Menu }
        "5" { Invoke-LocalScript "dev-check.ps1"; Pause-Menu }
        "6" { Invoke-LocalScript "dev-check.ps1" @("-Smoke"); Pause-Menu }
        "7" { Invoke-LocalScript "dev-check.ps1" @("-Smoke", "-SkipAsk"); Pause-Menu }
        "8" { Invoke-LocalScript "backup.ps1"; Pause-Menu }
        "9" { Invoke-RestoreFromMenu; Pause-Menu }
        "10" { Invoke-LocalScript "dev-down.ps1"; Pause-Menu }
        "11" {
          if (Confirm-Action "This removes local database, object, queue, cache, and vector volumes. Type REMOVE to continue" "REMOVE") {
            Invoke-LocalScript "dev-down.ps1" @("-Volumes")
          } else {
            Write-Host "Volume removal cancelled."
          }
          Pause-Menu
        }
        "12" { Invoke-LocalScript "test.ps1"; Pause-Menu }
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
