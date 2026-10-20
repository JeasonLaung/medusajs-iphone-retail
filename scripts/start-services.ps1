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

  $appName = if ($name -eq 'medusa') { 'backend' } else { 'storefront' }
  $lockFile = Join-Path $logsDir "dev-$appName.lock"
  $process = $null
  if (Test-Path -LiteralPath $lockFile) {
    $supervisorId = 0
    if ([int]::TryParse((Get-Content -LiteralPath $lockFile -Raw).Trim(), [ref]$supervisorId)) {
      $candidate = Get-CimInstance Win32_Process -Filter "ProcessId = $supervisorId" -ErrorAction SilentlyContinue
      if ($candidate.CommandLine -like "*dev-service.cjs*$appName*") {
        $process = Get-Process -Id $supervisorId -ErrorAction SilentlyContinue
      }
    }
  }
  if ($process) {
    Write-Host "$name supervisor is already running; waiting for readiness."
  } else {
    Write-Host "Starting $name on port $port..."
    $runnerPath = Join-Path $PSScriptRoot 'dev-service.cjs'
    $process = Start-Process -FilePath 'node' -ArgumentList @(('"' + $runnerPath + '"'), $appName) `
      -WorkingDirectory $workingDir -WindowStyle Hidden -PassThru `
      -RedirectStandardOutput (Join-Path $logsDir "$name.out.log") `
      -RedirectStandardError (Join-Path $logsDir "$name.err.log")
    Set-Content -Path (Join-Path $logsDir "$name.pid") -Value $process.Id
  }

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
  foreach ($command in @('docker', 'corepack', 'node')) {
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
  & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'start-device-camera.ps1')
  if ($LASTEXITCODE -ne 0) { Write-Warning 'Camera HTTPS startup failed. Run start-device-camera.bat to retry; see logs/device-camera.*.log.' }
  Start-App 'storefront' $storefrontDir 8000 'http://127.0.0.1:8000/dk/store'

  $frpConfig = Join-Path $projectDir 'frp\frpc.toml'
  if (Test-Path -LiteralPath $frpConfig) {
    $frpText = Get-Content -LiteralPath $frpConfig -Raw
    if ($frpText -notmatch 'YOUR_FRP_SERVER|YOUR_FRP_TOKEN' -and $frpText -match '(?m)^\s*\[\[proxies\]\]') {
      & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'start-frp.ps1')
      if ($LASTEXITCODE -ne 0) { throw 'frpc failed to start. See logs/frpc.*.log.' }
    } else {
      Write-Host 'frpc configuration is incomplete; skipping tunnel startup.'
    }
  }

  $lanAddress = (Get-NetIPAddress -AddressFamily IPv4 -PrefixOrigin Dhcp -ErrorAction SilentlyContinue |
    Where-Object { $_.IPAddress -notlike '169.254.*' -and $_.IPAddress -ne '127.0.0.1' } |
    Select-Object -First 1 -ExpandProperty IPAddress)
  Write-Host 'Admin (local): http://127.0.0.1:9001/app'
  if ($lanAddress) { Write-Host "Admin (LAN): http://$lanAddress`:9001/app" }
  Write-Host 'Store: http://127.0.0.1:8000/dk/store'
  exit 0
} catch {
  Write-Error $_.Exception.Message
  exit 1
}
