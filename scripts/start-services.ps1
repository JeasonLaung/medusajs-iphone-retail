$ErrorActionPreference = 'Stop'

$projectDir = Split-Path -Parent $PSScriptRoot
$workspaceDir = Join-Path $projectDir 'medusa-store'
$backendDir = Join-Path $workspaceDir 'apps\backend'
$storefrontDir = Join-Path $workspaceDir 'apps\storefront'
$logsDir = Join-Path $projectDir 'logs'

function Test-HttpOk([string]$url) {
  try {
    $response = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 5
    return $response.StatusCode -eq 200
  } catch {
    return $false
  }
}

function Test-PortListening([int]$port) {
  return [bool](Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue)
}

function Start-App([string]$name, [string]$workingDir, [int]$port, [string]$url) {
  if (Test-HttpOk $url) {
    Write-Host "$name is already running on port $port."
    return
  }

  if (Test-PortListening $port) {
    throw "Port $port is in use, but $name did not respond at $url."
  }

  Write-Host "Starting $name on port $port..."
  $process = Start-Process -FilePath 'cmd.exe' -ArgumentList '/d', '/c', 'corepack pnpm run dev' `
    -WorkingDirectory $workingDir -WindowStyle Hidden -PassThru `
    -RedirectStandardOutput (Join-Path $logsDir "$name.out.log") `
    -RedirectStandardError (Join-Path $logsDir "$name.err.log")
  Set-Content -Path (Join-Path $logsDir "$name.pid") -Value $process.Id

  for ($attempt = 0; $attempt -lt 90; $attempt++) {
    if (Test-HttpOk $url) {
      Write-Host "$name is ready."
      return
    }
    if ($process.HasExited) {
      break
    }
    Start-Sleep -Seconds 2
  }

  $errorLog = Join-Path $logsDir "$name.err.log"
  if (Test-Path $errorLog) {
    Get-Content $errorLog -Tail 20 | Write-Host
  }
  throw "$name did not become ready. See $logsDir."
}

try {
  foreach ($command in @('docker', 'corepack')) {
    if (-not (Get-Command $command -ErrorAction SilentlyContinue)) {
      throw "$command is not available in PATH."
    }
  }
  if (-not (Test-Path (Join-Path $workspaceDir 'node_modules'))) {
    throw "Dependencies are missing. Run corepack pnpm install in $workspaceDir."
  }

  New-Item -ItemType Directory -Path $logsDir -Force | Out-Null
  Push-Location $projectDir
  try {
    & docker compose up -d --wait
    if ($LASTEXITCODE -ne 0) { throw 'Docker Compose failed to start.' }
  } finally {
    Pop-Location
  }

  Start-App 'medusa' $backendDir 9001 'http://127.0.0.1:9001/health'
  Start-App 'storefront' $storefrontDir 8000 'http://127.0.0.1:8000/dk/store'

  Write-Host 'Admin: http://127.0.0.1:9001/app'
  Write-Host 'Store: http://127.0.0.1:8000/dk/store'
  exit 0
} catch {
  Write-Error $_.Exception.Message
  exit 1
}
