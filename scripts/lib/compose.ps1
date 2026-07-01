$script:NexusDeploymentProfiles = @("cpu-lite", "split-nas-gpu", "gpu-local", "prod-auth")
$script:NexusEmbeddingRuntimes = @("none", "external", "cpu", "gpu")

function Read-NexusEnvValue($path, $key) {
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

function Resolve-NexusDeploymentProfile {
  param(
    [string]$Profile = "",
    [string]$EnvFile = ""
  )

  $value = $Profile
  if ([string]::IsNullOrWhiteSpace($value) -and $EnvFile) {
    $value = Read-NexusEnvValue $EnvFile "DEPLOYMENT_PROFILE"
  }
  if ([string]::IsNullOrWhiteSpace($value)) {
    $value = "cpu-lite"
  }

  $normalized = $value.Trim().ToLowerInvariant()
  if ($script:NexusDeploymentProfiles -notcontains $normalized) {
    throw "Unknown deployment profile '$value'. Use one of: $($script:NexusDeploymentProfiles -join ', ')"
  }
  return $normalized
}

function Resolve-NexusEmbeddingRuntime {
  param(
    [string]$EnvFile = ""
  )

  $value = ""
  if ($EnvFile) {
    $value = Read-NexusEnvValue $EnvFile "EMBEDDING_RUNTIME"
  }
  if ([string]::IsNullOrWhiteSpace($value)) {
    $value = "none"
  }

  $normalized = $value.Trim().ToLowerInvariant()
  if ($script:NexusEmbeddingRuntimes -notcontains $normalized) {
    throw "Unknown embedding runtime '$value'. Use one of: $($script:NexusEmbeddingRuntimes -join ', ')"
  }
  return $normalized
}

function Test-NexusSourceMountEnabled {
  param(
    [string]$EnvFile = ""
  )

  if ([string]::IsNullOrWhiteSpace($EnvFile)) {
    return $false
  }
  $hostPath = Read-NexusEnvValue $EnvFile "NEXUS_SOURCE_HOST_PATH"
  return -not [string]::IsNullOrWhiteSpace($hostPath)
}

function Test-NexusUIShutdownEnabled {
  param(
    [string]$EnvFile = ""
  )

  if ([string]::IsNullOrWhiteSpace($EnvFile)) {
    return $false
  }
  $value = (Read-NexusEnvValue $EnvFile "UI_SHUTDOWN_ENABLED").Trim().ToLowerInvariant()
  return $value -in @("1", "true", "yes", "y", "on")
}

function Get-NexusComposeFiles {
  param(
    [string]$Profile,
    [string]$EmbeddingRuntime,
    [bool]$IncludeSourceMounts = $false,
    [bool]$IncludeOperatorControls = $false
  )

  $files = [System.Collections.Generic.List[string]]::new()
  switch ($Profile) {
    "cpu-lite" {
      $files.Add("deploy/compose/compose.cpu.yml")
    }
    "split-nas-gpu" {
      $files.Add("deploy/compose/compose.cpu.yml")
      $files.Add("deploy/compose/compose.split-nas-gpu.yml")
    }
    "gpu-local" {
      $files.Add("deploy/compose/compose.cpu.yml")
      $files.Add("deploy/compose/compose.gpu.yml")
    }
    "prod-auth" {
      $files.Add("deploy/compose/compose.prod-auth.yml")
    }
  }

  if ($EmbeddingRuntime -in @("cpu", "gpu")) {
    $files.Add("deploy/compose/compose.embeddings.yml")
  }
  if ($EmbeddingRuntime -eq "gpu") {
    $files.Add("deploy/compose/compose.embeddings.gpu.yml")
  }
  if ($IncludeSourceMounts) {
    $files.Add("deploy/compose/compose.sources.yml")
  }
  if ($IncludeOperatorControls) {
    $files.Add("deploy/compose/compose.control.yml")
  }
  return [string[]]$files
}

function Get-NexusComposeConfig {
  param(
    [string]$Root,
    [string]$EnvFile,
    [string]$Profile = ""
  )

  $selectedProfile = Resolve-NexusDeploymentProfile -Profile $Profile -EnvFile $EnvFile
  $embeddingRuntime = Resolve-NexusEmbeddingRuntime -EnvFile $EnvFile
  $includeSourceMounts = Test-NexusSourceMountEnabled -EnvFile $EnvFile
  $includeOperatorControls = Test-NexusUIShutdownEnabled -EnvFile $EnvFile
  $files = Get-NexusComposeFiles -Profile $selectedProfile -EmbeddingRuntime $embeddingRuntime -IncludeSourceMounts $includeSourceMounts -IncludeOperatorControls $includeOperatorControls
  $composeArgs = @("compose")

  if (Test-Path $EnvFile) {
    $composeArgs += @("--env-file", $EnvFile)
  }
  foreach ($file in $files) {
    $composeArgs += @("-f", (Join-Path $Root $file))
  }
  if ($selectedProfile -eq "gpu-local") {
    $composeArgs += @("--profile", "gpu")
  }

  return [pscustomobject]@{
    Args = [string[]]$composeArgs
    Profile = $selectedProfile
    EmbeddingRuntime = $embeddingRuntime
    SourceMounts = $includeSourceMounts
    OperatorControls = $includeOperatorControls
    Files = [string[]]$files
  }
}
