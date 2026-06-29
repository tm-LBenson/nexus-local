param(
  [Parameter(Mandatory = $true)]
  [string]$SourcePath,
  [string]$ApiUrl = "http://localhost:8080",
  [string]$TenantId = "",
  [string]$WorkspaceName = "",
  [string[]]$Extensions = @(".md", ".txt", ".json", ".html", ".htm", ".csv", ".tsv", ".vtt", ".pdf", ".docx", ".pptx", ".xlsx"),
  [string[]]$ExcludeDirectoryNames = @("node_modules", "venv", "__pycache__", "dist", "build"),
  [int]$Limit = 0,
  [int]$MaxFileMB = 60,
  [switch]$DryRun
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $root ".env"

function Read-EnvValue($path, $key) {
  if (-not (Test-Path -LiteralPath $path)) {
    return ""
  }
  foreach ($line in Get-Content -LiteralPath $path) {
    if ($line -match "^\s*$([regex]::Escape($key))=(.*)$") {
      return $matches[1].Trim()
    }
  }
  return ""
}

function Resolve-ApiUrl($url) {
  if ($url -ne "http://localhost:8080") {
    return $url.TrimEnd("/")
  }
  $apiPort = Read-EnvValue $envFile "API_HOST_PORT"
  if ($apiPort -and $apiPort -ne "8080") {
    return "http://localhost:$apiPort"
  }
  return $url.TrimEnd("/")
}

function Get-RelativePath($rootPath, $path) {
  $normalizedRoot = $rootPath.TrimEnd("\", "/")
  $relative = $path.Substring($normalizedRoot.Length).TrimStart("\", "/")
  return ($relative -replace "\\", "/")
}

function Test-ExcludedPath($relativePath, $excludedNames) {
  $segments = $relativePath -split "[\\/]+"
  foreach ($segment in $segments) {
    if ($segment.StartsWith(".")) {
      return $true
    }
    if ($excludedNames -contains $segment) {
      return $true
    }
  }
  return $false
}

function Get-ContentType($extension) {
  switch ($extension.ToLowerInvariant()) {
    ".md" { return "text/markdown" }
    ".txt" { return "text/plain" }
    ".json" { return "application/json" }
    ".html" { return "text/html" }
    ".htm" { return "text/html" }
    ".csv" { return "text/csv" }
    ".tsv" { return "text/tab-separated-values" }
    ".vtt" { return "text/vtt" }
    ".pdf" { return "application/pdf" }
    ".docx" { return "application/vnd.openxmlformats-officedocument.wordprocessingml.document" }
    ".pptx" { return "application/vnd.openxmlformats-officedocument.presentationml.presentation" }
    ".xlsx" { return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" }
    default { return "application/octet-stream" }
  }
}

function Select-TenantId($apiBase, $tenantId, $workspaceName) {
  if (-not [string]::IsNullOrWhiteSpace($tenantId)) {
    return $tenantId
  }

  $me = Invoke-RestMethod -Uri "$apiBase/v1/me" -TimeoutSec 10
  $memberships = @($me.memberships)
  if ($memberships.Count -eq 0) {
    throw "No workspace exists. Create a workspace in the web UI first."
  }

  if (-not [string]::IsNullOrWhiteSpace($workspaceName)) {
    $match = $memberships | Where-Object { $_.tenant.name -eq $workspaceName } | Select-Object -First 1
    if (-not $match) {
      throw "Workspace '$workspaceName' was not found."
    }
    return $match.tenant.id
  }

  return $memberships[0].tenant.id
}

function Send-Document($client, $apiBase, $tenantId, $file, $relativeName) {
  $stream = $null
  $form = $null
  try {
    $form = [System.Net.Http.MultipartFormDataContent]::new()
    $form.Add([System.Net.Http.StringContent]::new($tenantId), "tenant_id")
    $form.Add([System.Net.Http.StringContent]::new($relativeName), "name")

    $stream = [System.IO.File]::OpenRead($file.FullName)
    $fileContent = [System.Net.Http.StreamContent]::new($stream)
    $fileContent.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::Parse((Get-ContentType $file.Extension))
    $form.Add($fileContent, "file", $file.Name)

    $response = $client.PostAsync("$apiBase/v1/documents/upload", $form).GetAwaiter().GetResult()
    $body = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
    if (-not $response.IsSuccessStatusCode) {
      throw "upload failed ($([int]$response.StatusCode)): $body"
    }
    return $body | ConvertFrom-Json
  } finally {
    if ($form) {
      $form.Dispose()
    }
    if ($stream) {
      $stream.Dispose()
    }
  }
}

$apiBase = Resolve-ApiUrl $ApiUrl
$resolvedSource = Resolve-Path -LiteralPath $SourcePath
$sourceRoot = $resolvedSource.Path.TrimEnd("\", "/")
$maxBytes = [int64]$MaxFileMB * 1MB
$normalizedExtensions = @($Extensions | ForEach-Object { $_.ToLowerInvariant() })

$tenant = Select-TenantId $apiBase $TenantId $WorkspaceName
$files = Get-ChildItem -LiteralPath $sourceRoot -Recurse -File |
  ForEach-Object {
    $relative = Get-RelativePath $sourceRoot $_.FullName
    [pscustomobject]@{
      File = $_
      Relative = $relative
    }
  } |
  Where-Object {
    ($normalizedExtensions -contains $_.File.Extension.ToLowerInvariant()) -and
    (-not (Test-ExcludedPath $_.Relative $ExcludeDirectoryNames)) -and
    ($_.File.Length -le $maxBytes)
  } |
  Sort-Object Relative

if ($Limit -gt 0) {
  $files = @($files | Select-Object -First $Limit)
}

Write-Host "Nexus Local document folder import"
Write-Host "Source:    $sourceRoot"
Write-Host "API:       $apiBase"
Write-Host "Workspace: $tenant"
Write-Host "Files:     $($files.Count)"

if ($DryRun) {
  $files | Select-Object -First 50 | ForEach-Object { Write-Host "[DRY]  $($_.Relative)" }
  if ($files.Count -gt 50) {
    Write-Host "[DRY]  ... $($files.Count - 50) more"
  }
  return
}

$client = [System.Net.Http.HttpClient]::new()
$uploaded = 0
$failed = 0
try {
  foreach ($item in $files) {
    try {
      $result = Send-Document $client $apiBase $tenant $item.File $item.Relative
      $uploaded++
      Write-Host "[OK]   $($item.Relative) -> $($result.document.id)"
    } catch {
      $failed++
      Write-Host "[FAIL] $($item.Relative) - $($_.Exception.Message)"
    }
  }
} finally {
  $client.Dispose()
}

Write-Host ""
Write-Host "Uploaded: $uploaded"
Write-Host "Failed:   $failed"
if ($failed -gt 0) {
  throw "$failed file(s) failed to upload"
}
