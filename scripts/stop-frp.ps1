$ErrorActionPreference = 'Stop'
$projectDir = Split-Path -Parent $PSScriptRoot
$binary = Join-Path $projectDir 'frp\bin\frpc.exe'
$pidFile = Join-Path $projectDir 'logs\frpc.pid'
if (Test-Path -LiteralPath $pidFile) {
  $frpcPid = [int](Get-Content -LiteralPath $pidFile)
  $process = Get-CimInstance Win32_Process -Filter "ProcessId = $frpcPid" -ErrorAction SilentlyContinue
  if ($process -and $process.ExecutablePath -eq $binary) {
    Stop-Process -Id $frpcPid
    Write-Host 'frpc stopped.'
  }
  Remove-Item -LiteralPath $pidFile
}
$gatewayPidFile = Join-Path $projectDir 'logs\frp-gateway.pid'
if (Test-Path -LiteralPath $gatewayPidFile) {
  $gatewayPid = [int](Get-Content -LiteralPath $gatewayPidFile)
  $process = Get-CimInstance Win32_Process -Filter "ProcessId = $gatewayPid" -ErrorAction SilentlyContinue
  $gatewayScript = Join-Path $PSScriptRoot 'frp-gateway.cjs'
  if ($process -and $process.CommandLine -like "*$gatewayScript*") { Stop-Process -Id $gatewayPid }
  Remove-Item -LiteralPath $gatewayPidFile
}
