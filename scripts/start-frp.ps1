$ErrorActionPreference = 'Stop'
$projectDir = Split-Path -Parent $PSScriptRoot
$binary = Join-Path $projectDir 'frp\bin\frpc.exe'
$config = Join-Path $projectDir 'frp\frpc.toml'
$logsDir = Join-Path $projectDir 'logs'
$pidFile = Join-Path $logsDir 'frpc.pid'

if (-not (Test-Path -LiteralPath $config)) { throw 'Create frp\frpc.toml using frpc.example.toml first.' }
$configText = Get-Content -LiteralPath $config -Raw
if ($configText -match 'YOUR_FRP_SERVER|YOUR_FRP_TOKEN' -or $configText -notmatch '(?m)^\s*\[\[proxies\]\]') {
  throw 'Fill in the server address, token, and at least one proxy in frp\frpc.toml.'
}
if (Test-Path -LiteralPath $pidFile) {
  $existingPid = [int](Get-Content -LiteralPath $pidFile)
  $existing = Get-CimInstance Win32_Process -Filter "ProcessId = $existingPid" -ErrorAction SilentlyContinue
  if ($existing -and $existing.ExecutablePath -eq $binary) {
    Write-Host "frpc is already running (PID $existingPid)."
    exit 0
  }
}
if (-not (Test-Path -LiteralPath $binary)) {
  & (Join-Path $PSScriptRoot 'install-frp.ps1')
}
& $binary verify -c $config
if ($LASTEXITCODE -ne 0) { throw 'frpc configuration validation failed.' }
New-Item -ItemType Directory -Path $logsDir -Force | Out-Null
$gatewayScript = Join-Path $PSScriptRoot 'frp-gateway.cjs'
$gatewayPidFile = Join-Path $logsDir 'frp-gateway.pid'
$listener = Get-NetTCPConnection -LocalPort 18080 -State Listen -ErrorAction SilentlyContinue
if ($listener) {
  $gatewayPid = if (Test-Path -LiteralPath $gatewayPidFile) { [int](Get-Content -LiteralPath $gatewayPidFile) } else { 0 }
  $gatewayProcess = Get-CimInstance Win32_Process -Filter "ProcessId = $gatewayPid" -ErrorAction SilentlyContinue
  if ($listener.OwningProcess -ne $gatewayPid -or -not $gatewayProcess -or $gatewayProcess.CommandLine -notlike "*$gatewayScript*") {
    throw 'Port 18080 is occupied by another process.'
  }
} else {
  $nodePath = (Get-Command node -ErrorAction Stop).Source
  $gateway = Start-Process -FilePath $nodePath -ArgumentList ('"' + $gatewayScript + '"') `
    -WorkingDirectory $projectDir -WindowStyle Hidden -PassThru `
    -RedirectStandardOutput (Join-Path $logsDir 'frp-gateway.out.log') `
    -RedirectStandardError (Join-Path $logsDir 'frp-gateway.err.log')
  Set-Content -LiteralPath $gatewayPidFile -Value $gateway.Id
  Start-Sleep -Seconds 1
  if ($gateway.HasExited) { throw 'Local frp gateway failed. Check logs/frp-gateway.err.log.' }
}
$process = Start-Process -FilePath $binary -ArgumentList '-c', ('"' + $config + '"') `
  -WorkingDirectory $projectDir -WindowStyle Hidden -PassThru `
  -RedirectStandardOutput (Join-Path $logsDir 'frpc.out.log') `
  -RedirectStandardError (Join-Path $logsDir 'frpc.err.log')
Set-Content -LiteralPath $pidFile -Value $process.Id
for ($attempt = 0; $attempt -lt 15; $attempt++) {
  Start-Sleep -Seconds 2
  if ($process.HasExited) { throw 'frpc exited. Check logs/frpc.out.log and logs/frpc.err.log.' }
  $output = Get-Content -LiteralPath (Join-Path $logsDir 'frpc.out.log') -Raw -ErrorAction SilentlyContinue
  if ($output -match 'start error:') {
    $output -split "`n" | Where-Object { $_ -match 'start error:' } | Write-Host
    throw 'frpc is running, but the server rejected a proxy. Fix the server configuration; frpc will retry.'
  }
  $proxyCount = [regex]::Matches($configText, '(?m)^\s*\[\[proxies\]\]').Count
  if ([regex]::Matches([string]$output, 'start proxy success').Count -ge $proxyCount) {
    Write-Host "frpc connected and all proxies registered (PID $($process.Id))."
    exit 0
  }
}
Write-Warning "frpc is running (PID $($process.Id)), but proxy registration is not confirmed. Check logs/frpc.out.log."
