param(
  [string]$Profile = "",
  [string]$OutputPath = "",
  [switch]$Force,
  [switch]$NonInteractive,
  [switch]$SkipPortCheck,
  [string]$ProviderPreset = "",
  [string]$ApiHostPort = "",
  [string]$WebHostPort = "",
  [string]$PublicUrl = "",
  [string]$ModelGatewayBaseUrl = "",
  [string]$ModelGatewayPort = "",
  [string]$ModelGatewayApiKey = "",
  [string]$EmbeddingRuntime = "",
  [string]$EmbeddingBaseUrl = "",
  [string]$EmbeddingBackend = "",
  [string]$EmbeddingApiKey = "",
  [string]$EmbeddingGatewayImage = "",
  [string]$EmbeddingGatewayPort = "",
  [string]$EmbeddingModel = "",
  [string]$EmbeddingDimensions = "",
  [string]$GeneralModelId = "",
  [string]$AutheliaInternalUrl = "",
  [string]$NexusHttpPort = "",
  [string]$NexusHttpsPort = "",
  [string]$PostgresPassword = "",
  [string]$ObjectStorageAccessKey = "",
  [string]$ObjectStorageSecretKey = ""
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$templatePath = Join-Path $root ".env.example"
if (-not $OutputPath) {
  $OutputPath = Join-Path $root ".env"
}

$profileNames = @("cpu-lite", "split-nas-gpu", "gpu-local", "prod-auth")
$providerPresetNames = @("starter", "semantic")
$providerPresetDescriptions = @{
  "starter" = "OpenAI-compatible chat plus local hash embeddings"
  "semantic" = "OpenAI-compatible chat plus OpenAI-compatible embeddings"
}
$embeddingRuntimeNames = @("none", "external", "cpu", "gpu")
$embeddingRuntimeDescriptions = @{
  "none" = "Use hash embeddings; do not start an embedding service"
  "external" = "Use an existing OpenAI-compatible embedding endpoint"
  "cpu" = "Start the self-hosted TEI CPU embedding service"
  "gpu" = "Start the self-hosted TEI GPU embedding service"
}

function New-RandomToken([int]$ByteLength = 24) {
  $bytes = [byte[]]::new($ByteLength)
  [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
  return -join ($bytes | ForEach-Object { $_.ToString("x2") })
}

function Read-TemplateEnv($path) {
  $values = [ordered]@{}
  foreach ($line in Get-Content $path) {
    if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$') {
      $values[$matches[1]] = $matches[2]
    }
  }
  return $values
}

function Set-EnvValue($values, $key, $value) {
  $values[$key] = $value
}

function Get-ProvidedOrDefault($provided, $default) {
  if (-not [string]::IsNullOrWhiteSpace($provided)) {
    return $provided.Trim()
  }
  return $default
}

function Read-SetupValue($label, $default, $provided = "") {
  if (-not [string]::IsNullOrWhiteSpace($provided)) {
    return $provided.Trim()
  }
  if ($NonInteractive) {
    return $default
  }
  $answer = Read-Host "$label [$default]"
  if ([string]::IsNullOrWhiteSpace($answer)) {
    return $default
  }
  return $answer.Trim()
}

function Select-SetupProfile {
  if (-not [string]::IsNullOrWhiteSpace($Profile)) {
    $normalized = $Profile.Trim().ToLowerInvariant()
    if ($profileNames -notcontains $normalized) {
      throw "Unknown profile '$Profile'. Use one of: $($profileNames -join ', ')"
    }
    return $normalized
  }
  if ($NonInteractive) {
    return "cpu-lite"
  }

  Write-Host "Choose a deployment profile:"
  for ($i = 0; $i -lt $profileNames.Count; $i++) {
    Write-Host "  $($i + 1). $($profileNames[$i])"
  }
  while ($true) {
    $choice = Read-Host "Profile [cpu-lite]"
    if ([string]::IsNullOrWhiteSpace($choice)) {
      return "cpu-lite"
    }
    $choice = $choice.Trim().ToLowerInvariant()
    if ($choice -match '^\d+$') {
      $index = [int]$choice - 1
      if ($index -ge 0 -and $index -lt $profileNames.Count) {
        return $profileNames[$index]
      }
    }
    if ($profileNames -contains $choice) {
      return $choice
    }
    Write-Host "Use one of: $($profileNames -join ', ')"
  }
}

function Select-ProviderPreset {
  $defaultPreset = "starter"
  if (-not [string]::IsNullOrWhiteSpace($ProviderPreset)) {
    $normalized = $ProviderPreset.Trim().ToLowerInvariant()
    if ($providerPresetNames -notcontains $normalized) {
      throw "Unknown provider preset '$ProviderPreset'. Use one of: $($providerPresetNames -join ', ')"
    }
    return $normalized
  }
  if ($NonInteractive) {
    return $defaultPreset
  }

  Write-Host "Choose a provider preset:"
  for ($i = 0; $i -lt $providerPresetNames.Count; $i++) {
    $name = $providerPresetNames[$i]
    Write-Host "  $($i + 1). $name - $($providerPresetDescriptions[$name])"
  }
  while ($true) {
    $choice = Read-Host "Provider preset [$defaultPreset]"
    if ([string]::IsNullOrWhiteSpace($choice)) {
      return $defaultPreset
    }
    $choice = $choice.Trim().ToLowerInvariant()
    if ($choice -match '^\d+$') {
      $index = [int]$choice - 1
      if ($index -ge 0 -and $index -lt $providerPresetNames.Count) {
        return $providerPresetNames[$index]
      }
    }
    if ($providerPresetNames -contains $choice) {
      return $choice
    }
    Write-Host "Use one of: $($providerPresetNames -join ', ')"
  }
}

function Select-EmbeddingRuntime($defaultRuntime) {
  if (-not [string]::IsNullOrWhiteSpace($EmbeddingRuntime)) {
    $normalized = $EmbeddingRuntime.Trim().ToLowerInvariant()
    if ($embeddingRuntimeNames -notcontains $normalized) {
      throw "Unknown embedding runtime '$EmbeddingRuntime'. Use one of: $($embeddingRuntimeNames -join ', ')"
    }
    return $normalized
  }
  if ($NonInteractive) {
    return $defaultRuntime
  }

  Write-Host "Choose an embedding runtime:"
  for ($i = 0; $i -lt $embeddingRuntimeNames.Count; $i++) {
    $name = $embeddingRuntimeNames[$i]
    Write-Host "  $($i + 1). $name - $($embeddingRuntimeDescriptions[$name])"
  }
  while ($true) {
    $choice = Read-Host "Embedding runtime [$defaultRuntime]"
    if ([string]::IsNullOrWhiteSpace($choice)) {
      return $defaultRuntime
    }
    $choice = $choice.Trim().ToLowerInvariant()
    if ($choice -match '^\d+$') {
      $index = [int]$choice - 1
      if ($index -ge 0 -and $index -lt $embeddingRuntimeNames.Count) {
        return $embeddingRuntimeNames[$index]
      }
    }
    if ($embeddingRuntimeNames -contains $choice) {
      return $choice
    }
    Write-Host "Use one of: $($embeddingRuntimeNames -join ', ')"
  }
}

function Normalize-EmbeddingBackend($backend) {
  $normalized = $backend.Trim().ToLowerInvariant()
  switch ($normalized) {
    "" { return "hash" }
    "hash" { return "hash" }
    "openai" { return "openai-compatible" }
    "openai-compatible" { return "openai-compatible" }
    "openaicompat" { return "openai-compatible" }
    "tei" { return "tei" }
    default {
      throw "Unknown embedding backend '$backend'. Use hash, openai-compatible, or tei."
    }
  }
}

function Assert-PositiveInteger($label, $value) {
  $parsed = 0
  if (-not [int]::TryParse($value, [ref]$parsed) -or $parsed -le 0) {
    throw "$label must be a positive integer."
  }
  return $parsed.ToString()
}

function Get-SiteAddress($publicUrl) {
  try {
    $uri = [Uri]$publicUrl
    if ($uri.Host -and $uri.Host -notin @("localhost", "127.0.0.1")) {
      if (($uri.Scheme -eq "http" -and $uri.Port -ne 80) -or ($uri.Scheme -eq "https" -and $uri.Port -ne 443)) {
        return "$($uri.Host):$($uri.Port)"
      }
      return $uri.Host
    }
  } catch {
    return ":80"
  }
  return ":80"
}

function Test-PortOpen($port) {
  try {
    $client = [System.Net.Sockets.TcpClient]::new()
    $async = $client.BeginConnect("127.0.0.1", $port, $null, $null)
    if (-not $async.AsyncWaitHandle.WaitOne(350)) {
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

function Test-Ports($ports) {
  if ($SkipPortCheck) {
    return
  }
  foreach ($port in $ports) {
    if (Test-PortOpen $port) {
      Write-Host "[WARN] Port $port already has a listener."
    }
  }
}

function Test-SetupTools {
  if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Host "[WARN] Docker was not found on PATH. Install Docker before starting the container profile."
    return $false
  }
  try {
    & docker compose version | Out-Null
    return $true
  } catch {
    Write-Host "[WARN] Docker Compose is not available through 'docker compose'."
    return $false
  }
}

function Write-EnvFile($path, $values) {
  $sections = @(
    @{ Title = "App"; Keys = @(
        "APP_ENV", "DEPLOYMENT_PROFILE", "HTTP_ADDR", "API_HOST_PORT", "WEB_HOST_PORT", "APP_VERSION", "CORS_ALLOWED_ORIGIN",
        "AUTH_MODE", "DEV_USER_ID", "DEV_USER_EMAIL",
        "TRUSTED_USER_ID_HEADER", "TRUSTED_EMAIL_HEADER"
      )
    },
    @{ Title = "Persistence"; Keys = @(
        "PERSISTENCE_BACKEND", "RUN_MIGRATIONS", "POSTGRES_USER",
        "POSTGRES_PASSWORD", "POSTGRES_DB", "DATABASE_URL"
      )
    },
    @{ Title = "Storage and Retrieval"; Keys = @(
        "OBJECT_STORAGE_BACKEND", "OBJECT_STORAGE_ENDPOINT",
        "OBJECT_STORAGE_ACCESS_KEY", "OBJECT_STORAGE_SECRET_KEY",
        "OBJECT_STORAGE_BUCKET", "EMBEDDING_RUNTIME", "EMBEDDING_BACKEND",
        "EMBEDDING_BASE_URL", "EMBEDDING_API_KEY", "EMBEDDING_GATEWAY_IMAGE",
        "EMBEDDING_GATEWAY_PORT", "EMBEDDING_MODEL", "EMBEDDING_DIMENSIONS",
        "VECTOR_BACKEND", "VECTOR_BASE_URL", "VECTOR_COLLECTION", "VECTOR_API_KEY",
        "QUEUE_BACKEND", "QUEUE_URL", "CACHE_URL"
      )
    },
    @{ Title = "Models"; Keys = @(
        "PROVIDER_PRESET", "MODEL_GATEWAY_BASE_URL", "MODEL_GATEWAY_PORT", "MODEL_GATEWAY_API_KEY",
        "DEFAULT_MODEL_TARGET", "GENERAL_MODEL_ID", "HUGGING_FACE_HUB_TOKEN",
        "WORKER_POLL_INTERVAL"
      )
    },
    @{ Title = "Web and Production Auth"; Keys = @(
        "VITE_API_BASE_URL", "NEXUS_PUBLIC_URL", "NEXUS_SITE_ADDRESS",
        "NEXUS_HTTP_PORT", "NEXUS_HTTPS_PORT", "WEB_API_BASE_URL",
        "AUTHELIA_INTERNAL_URL"
      )
    }
  )

  $written = [System.Collections.Generic.HashSet[string]]::new()
  $lines = [System.Collections.Generic.List[string]]::new()
  $lines.Add("# Generated by scripts/setup.ps1")
  $lines.Add("# Review secrets and URLs before deploying outside local development.")
  $lines.Add("")

  foreach ($section in $sections) {
    $lines.Add("# $($section.Title)")
    foreach ($key in $section.Keys) {
      if ($values.Contains($key)) {
        $lines.Add("$key=$($values[$key])")
        [void]$written.Add($key)
      }
    }
    $lines.Add("")
  }

  $extraKeys = @($values.Keys | Where-Object { -not $written.Contains($_) } | Sort-Object)
  if ($extraKeys.Count -gt 0) {
    $lines.Add("# Other")
    foreach ($key in $extraKeys) {
      $lines.Add("$key=$($values[$key])")
    }
    $lines.Add("")
  }

  $parent = Split-Path -Parent $path
  if ($parent -and -not (Test-Path $parent)) {
    New-Item -ItemType Directory -Path $parent | Out-Null
  }
  Set-Content -Path $path -Value $lines -Encoding ascii
}

function Get-DisplayPath($path) {
  $fullPath = [System.IO.Path]::GetFullPath($path)
  $rootPath = [System.IO.Path]::GetFullPath($root).TrimEnd([System.IO.Path]::DirectorySeparatorChar, [System.IO.Path]::AltDirectorySeparatorChar)
  if ($fullPath.StartsWith($rootPath, [System.StringComparison]::OrdinalIgnoreCase)) {
    $relative = Resolve-Path -Path $fullPath -Relative
    if ($relative.StartsWith(".\") -or $relative.StartsWith("./")) {
      return $relative.Substring(2)
    }
    return $relative
  }
  return $fullPath
}

function Get-ComposeCommand($selectedProfile, $envPath) {
  $envArg = "--env-file `"$envPath`""
  $fileArgs = (Get-ComposeFiles $selectedProfile $embeddingRuntimeValue | ForEach-Object { "-f $_" }) -join " "
  $profileArg = ""
  if ($selectedProfile -eq "gpu-local") {
    $profileArg = " --profile gpu"
  }
  return "docker compose $envArg $fileArgs$profileArg up -d --build"
}

function Get-ComposeFiles($selectedProfile, $embeddingRuntime) {
  $files = [System.Collections.Generic.List[string]]::new()
  switch ($selectedProfile) {
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
  if ($embeddingRuntime -in @("cpu", "gpu")) {
    $files.Add("deploy/compose/compose.embeddings.yml")
  }
  if ($embeddingRuntime -eq "gpu") {
    $files.Add("deploy/compose/compose.embeddings.gpu.yml")
  }
  return $files
}

function Test-ComposeConfig($selectedProfile, $envPath) {
  $args = @("--env-file", $envPath)
  foreach ($file in (Get-ComposeFiles $selectedProfile $embeddingRuntimeValue)) {
    $args += @("-f", (Join-Path $root $file))
  }
  if ($selectedProfile -eq "gpu-local") {
    $args += @("--profile", "gpu")
  }
  $args += @("config", "--quiet")
  & docker compose @args
  if ($LASTEXITCODE -ne 0) {
    throw "docker compose config failed with exit code $LASTEXITCODE"
  }
}

if (-not (Test-Path $templatePath)) {
  throw ".env.example was not found at $templatePath"
}

$selectedProfile = Select-SetupProfile
$providerPresetValue = Select-ProviderPreset
$values = Read-TemplateEnv $templatePath

$postgresUser = Read-SetupValue "Postgres user" "app"
$postgresDb = Read-SetupValue "Postgres database" "app"
$postgresPasswordValue = Read-SetupValue "Postgres password" (Get-ProvidedOrDefault $PostgresPassword (New-RandomToken 18)) $PostgresPassword
$objectAccessValue = Read-SetupValue "Object storage access key" (Get-ProvidedOrDefault $ObjectStorageAccessKey "nexusadmin") $ObjectStorageAccessKey
$objectSecretValue = Read-SetupValue "Object storage secret key" (Get-ProvidedOrDefault $ObjectStorageSecretKey (New-RandomToken 18)) $ObjectStorageSecretKey
$modelValue = Read-SetupValue "General model ID" (Get-ProvidedOrDefault $GeneralModelId "Qwen/Qwen2.5-7B-Instruct") $GeneralModelId

$apiHostPortValue = Assert-PositiveInteger "API host port" (Get-ProvidedOrDefault $ApiHostPort "8080")
$webHostPortValue = Assert-PositiveInteger "Web host port" (Get-ProvidedOrDefault $WebHostPort "5173")
$defaultModelGateway = "http://host.docker.internal:8000/v1"
$modelGatewayPortValue = Get-ProvidedOrDefault $ModelGatewayPort "8000"
$defaultEmbeddingGateway = "http://host.docker.internal:8082/v1"
$defaultPublicUrl = "http://localhost:$webHostPortValue"
$defaultWebApiBase = "http://localhost:$apiHostPortValue"
$defaultAppEnv = "local"
$defaultAuthMode = "dev"
$nexusHttpPortValue = "8088"
$nexusHttpsPortValue = "8443"
$portsToCheck = @([int]$webHostPortValue, [int]$apiHostPortValue, 9000, 9001, 6333, 5432, 4222, 6379)

switch ($selectedProfile) {
  "split-nas-gpu" {
    $defaultModelGateway = "http://desktop-gpu.local:8000/v1"
    $defaultEmbeddingGateway = "http://desktop-gpu.local:8082/v1"
  }
  "gpu-local" {
    $defaultModelGateway = "http://model-gateway:8000/v1"
    $defaultEmbeddingGateway = "http://model-gateway:8000/v1"
  }
  "prod-auth" {
    $defaultPublicUrl = "http://localhost:8088"
    $defaultWebApiBase = "/api"
    $defaultAppEnv = "production"
    $defaultAuthMode = "trusted-header"
    $portsToCheck = @(8088, 8443)
  }
}

$defaultEmbeddingRuntime = "none"
if ($providerPresetValue -eq "semantic") {
  $defaultEmbeddingRuntime = "cpu"
}
if (-not [string]::IsNullOrWhiteSpace($EmbeddingBaseUrl)) {
  $defaultEmbeddingRuntime = "external"
}
$embeddingRuntimeValue = Select-EmbeddingRuntime $defaultEmbeddingRuntime
if ($providerPresetValue -eq "starter" -and $embeddingRuntimeValue -ne "none") {
  throw "Embedding runtime '$embeddingRuntimeValue' requires -ProviderPreset semantic."
}
if ($providerPresetValue -eq "semantic" -and $embeddingRuntimeValue -eq "none") {
  throw "Provider preset semantic requires embedding runtime external, cpu, or gpu."
}
if ($embeddingRuntimeValue -in @("cpu", "gpu")) {
  $defaultEmbeddingGateway = "http://embedding-gateway:80/v1"
}
$defaultEmbeddingGatewayImage = "ghcr.io/huggingface/text-embeddings-inference:cpu-1.9"
if ($embeddingRuntimeValue -eq "gpu") {
  $defaultEmbeddingGatewayImage = "ghcr.io/huggingface/text-embeddings-inference:86-1.9"
}

$publicUrlValue = Read-SetupValue "Public URL" (Get-ProvidedOrDefault $PublicUrl $defaultPublicUrl) $PublicUrl
$modelGatewayValue = Read-SetupValue "Model gateway base URL" (Get-ProvidedOrDefault $ModelGatewayBaseUrl $defaultModelGateway) $ModelGatewayBaseUrl
if ($selectedProfile -eq "gpu-local") {
  $modelGatewayPortValue = Assert-PositiveInteger "Model gateway host port" (Read-SetupValue "Model gateway host port" $modelGatewayPortValue $ModelGatewayPort)
  $portsToCheck += [int]$modelGatewayPortValue
}
$modelGatewayAPIKeyValue = Read-SetupValue "Model gateway API key" (Get-ProvidedOrDefault $ModelGatewayApiKey "") $ModelGatewayApiKey
$embeddingGatewayValue = Read-SetupValue "Embedding base URL" (Get-ProvidedOrDefault $EmbeddingBaseUrl $defaultEmbeddingGateway) $EmbeddingBaseUrl
$embeddingGatewayImageValue = Get-ProvidedOrDefault $EmbeddingGatewayImage $defaultEmbeddingGatewayImage
$embeddingGatewayPortValue = Assert-PositiveInteger "Embedding gateway port" (Get-ProvidedOrDefault $EmbeddingGatewayPort "8082")
if ($embeddingRuntimeValue -in @("cpu", "gpu")) {
  $embeddingGatewayImageValue = Read-SetupValue "Embedding gateway image" $embeddingGatewayImageValue $EmbeddingGatewayImage
  $embeddingGatewayPortValue = Assert-PositiveInteger "Embedding gateway port" (Read-SetupValue "Embedding gateway host port" $embeddingGatewayPortValue $EmbeddingGatewayPort)
}
$defaultEmbeddingBackend = "hash"
$defaultEmbeddingDimensions = "384"
if ($providerPresetValue -eq "semantic") {
  $defaultEmbeddingBackend = "openai-compatible"
}
$embeddingBackendValue = Normalize-EmbeddingBackend (Read-SetupValue "Embedding backend" (Get-ProvidedOrDefault $EmbeddingBackend $defaultEmbeddingBackend) $EmbeddingBackend)
$defaultEmbeddingModel = "hash-embedding"
if ($embeddingBackendValue -ne "hash") {
  $defaultEmbeddingModel = "BAAI/bge-small-en-v1.5"
}
$embeddingModelValue = Read-SetupValue "Embedding model" (Get-ProvidedOrDefault $EmbeddingModel $defaultEmbeddingModel) $EmbeddingModel
$embeddingDimensionsValue = Assert-PositiveInteger "Embedding dimensions" (Read-SetupValue "Embedding dimensions" (Get-ProvidedOrDefault $EmbeddingDimensions $defaultEmbeddingDimensions) $EmbeddingDimensions)
$embeddingAPIKeyValue = Read-SetupValue "Embedding API key" (Get-ProvidedOrDefault $EmbeddingApiKey "") $EmbeddingApiKey
$autheliaValue = "http://authelia:9091"
if ($selectedProfile -eq "prod-auth") {
  $autheliaValue = Read-SetupValue "Forward-auth internal URL" (Get-ProvidedOrDefault $AutheliaInternalUrl "http://authelia:9091") $AutheliaInternalUrl
  $nexusHttpPortValue = Read-SetupValue "Caddy HTTP host port" (Get-ProvidedOrDefault $NexusHttpPort "8088") $NexusHttpPort
  $nexusHttpsPortValue = Read-SetupValue "Caddy HTTPS host port" (Get-ProvidedOrDefault $NexusHttpsPort "8443") $NexusHttpsPort
  $portsToCheck = @()
  foreach ($portText in @($nexusHttpPortValue, $nexusHttpsPortValue)) {
    $parsedPort = 0
    if ([int]::TryParse($portText, [ref]$parsedPort)) {
      $portsToCheck += $parsedPort
    }
  }
}

Set-EnvValue $values "APP_ENV" $defaultAppEnv
Set-EnvValue $values "DEPLOYMENT_PROFILE" $selectedProfile
Set-EnvValue $values "HTTP_ADDR" ":8080"
Set-EnvValue $values "API_HOST_PORT" $apiHostPortValue
Set-EnvValue $values "WEB_HOST_PORT" $webHostPortValue
Set-EnvValue $values "CORS_ALLOWED_ORIGIN" $publicUrlValue
Set-EnvValue $values "AUTH_MODE" $defaultAuthMode
Set-EnvValue $values "DEV_USER_ID" "user_1"
Set-EnvValue $values "DEV_USER_EMAIL" "dev@example.local"
Set-EnvValue $values "TRUSTED_USER_ID_HEADER" "X-User-ID"
Set-EnvValue $values "TRUSTED_EMAIL_HEADER" "X-User-Email"
Set-EnvValue $values "PERSISTENCE_BACKEND" "postgres"
Set-EnvValue $values "RUN_MIGRATIONS" "true"
Set-EnvValue $values "POSTGRES_USER" $postgresUser
Set-EnvValue $values "POSTGRES_PASSWORD" $postgresPasswordValue
Set-EnvValue $values "POSTGRES_DB" $postgresDb
Set-EnvValue $values "DATABASE_URL" "postgres://$postgresUser`:$postgresPasswordValue@postgres:5432/$postgresDb`?sslmode=disable"
Set-EnvValue $values "OBJECT_STORAGE_BACKEND" "minio"
Set-EnvValue $values "OBJECT_STORAGE_ENDPOINT" "http://minio:9000"
Set-EnvValue $values "OBJECT_STORAGE_ACCESS_KEY" $objectAccessValue
Set-EnvValue $values "OBJECT_STORAGE_SECRET_KEY" $objectSecretValue
Set-EnvValue $values "OBJECT_STORAGE_BUCKET" "documents"
Set-EnvValue $values "EMBEDDING_RUNTIME" $embeddingRuntimeValue
Set-EnvValue $values "EMBEDDING_BACKEND" $embeddingBackendValue
Set-EnvValue $values "EMBEDDING_BASE_URL" $embeddingGatewayValue
Set-EnvValue $values "EMBEDDING_API_KEY" $embeddingAPIKeyValue
Set-EnvValue $values "EMBEDDING_GATEWAY_IMAGE" $embeddingGatewayImageValue
Set-EnvValue $values "EMBEDDING_GATEWAY_PORT" $embeddingGatewayPortValue
Set-EnvValue $values "EMBEDDING_MODEL" $embeddingModelValue
Set-EnvValue $values "EMBEDDING_DIMENSIONS" $embeddingDimensionsValue
Set-EnvValue $values "VECTOR_BACKEND" "qdrant"
Set-EnvValue $values "VECTOR_BASE_URL" "http://qdrant:6333"
Set-EnvValue $values "VECTOR_COLLECTION" "documents"
Set-EnvValue $values "VECTOR_API_KEY" ""
Set-EnvValue $values "QUEUE_BACKEND" "nats"
Set-EnvValue $values "QUEUE_URL" "nats://nats:4222"
Set-EnvValue $values "CACHE_URL" "redis://valkey:6379/0"
Set-EnvValue $values "PROVIDER_PRESET" $providerPresetValue
Set-EnvValue $values "MODEL_GATEWAY_BASE_URL" $modelGatewayValue
Set-EnvValue $values "MODEL_GATEWAY_PORT" $modelGatewayPortValue
Set-EnvValue $values "MODEL_GATEWAY_API_KEY" $modelGatewayAPIKeyValue
Set-EnvValue $values "DEFAULT_MODEL_TARGET" "general"
Set-EnvValue $values "GENERAL_MODEL_ID" $modelValue
Set-EnvValue $values "HUGGING_FACE_HUB_TOKEN" ""
Set-EnvValue $values "WORKER_POLL_INTERVAL" "2s"
Set-EnvValue $values "VITE_API_BASE_URL" $defaultWebApiBase
Set-EnvValue $values "NEXUS_PUBLIC_URL" $publicUrlValue
Set-EnvValue $values "NEXUS_SITE_ADDRESS" (Get-SiteAddress $publicUrlValue)
Set-EnvValue $values "NEXUS_HTTP_PORT" $nexusHttpPortValue
Set-EnvValue $values "NEXUS_HTTPS_PORT" $nexusHttpsPortValue
Set-EnvValue $values "WEB_API_BASE_URL" $defaultWebApiBase
Set-EnvValue $values "AUTHELIA_INTERNAL_URL" $autheliaValue

if ($selectedProfile -eq "prod-auth") {
  Set-EnvValue $values "VITE_API_BASE_URL" "/api"
  Set-EnvValue $values "WEB_API_BASE_URL" "/api"
}

if ((Test-Path $OutputPath) -and -not $Force) {
  if ($NonInteractive) {
    throw "$OutputPath already exists. Pass -Force to overwrite it."
  }
  $answer = Read-Host "$OutputPath already exists. Overwrite? [y/N]"
  if ($answer.Trim().ToLowerInvariant() -notin @("y", "yes")) {
    Write-Host "Setup canceled."
    exit 0
  }
}

Write-EnvFile $OutputPath $values
Write-Host "Wrote $OutputPath"

Test-Ports $portsToCheck
$hasCompose = Test-SetupTools
if ($hasCompose) {
  Test-ComposeConfig $selectedProfile $OutputPath
  Write-Host "Compose config validated for $selectedProfile."
}

$displayOutput = Get-DisplayPath $OutputPath
$nextCommand = Get-ComposeCommand $selectedProfile $displayOutput

Write-Host ""
Write-Host "Next command:"
Write-Host $nextCommand

if ($selectedProfile -eq "prod-auth") {
  Write-Host ""
  Write-Host "Production auth requires an Authelia-compatible forward-auth service at AUTHELIA_INTERNAL_URL."
}
