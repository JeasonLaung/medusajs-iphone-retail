param([int]$Port = 9443)
$ErrorActionPreference = 'Stop'
$projectDir = Split-Path -Parent $PSScriptRoot
$tlsDir = Join-Path $projectDir '.device-camera-tls'
$logDir = Join-Path $projectDir 'logs'
$opensslPath = 'C:\Program Files\Git\usr\bin\openssl.exe'
if (-not (Test-Path -LiteralPath $opensslPath)) {
  $opensslPath = (Get-Command openssl -ErrorAction Stop).Source
}
$addresses = @(Get-NetIPAddress -AddressFamily IPv4 | Where-Object {
  $_.IPAddress -match '^192\.168\.|^10\.|^172\.(1[6-9]|2[0-9]|3[01])\.' -and $_.InterfaceAlias -notmatch 'vEthernet|Loopback|WSL' -and $_.AddressState -eq 'Preferred'
} | Select-Object -ExpandProperty IPAddress)
if ($addresses.Count -eq 0) { throw 'No private LAN IPv4 address found. Connect to the same Wi-Fi / LAN as the phone.' }
New-Item -ItemType Directory -Path $tlsDir, $logDir -Force | Out-Null
$addressSignature = $addresses -join ','
$signaturePath = Join-Path $tlsDir 'addresses.txt'
$regenerate = -not (Test-Path -LiteralPath (Join-Path $tlsDir 'server.crt')) -or -not (Test-Path -LiteralPath $signaturePath)
if (-not $regenerate) { $regenerate = (Get-Content -LiteralPath $signaturePath -Raw).Trim() -ne $addressSignature }
function Invoke-OpenSsl([string[]]$Arguments) {
  $previousPreference = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  & $opensslPath @Arguments 2>> (Join-Path $logDir 'device-camera-certificate.log')
  $ErrorActionPreference = $previousPreference
  if ($LASTEXITCODE -ne 0) { throw 'Certificate generation failed. See logs/device-camera-certificate.log.' }
}
if (-not (Test-Path -LiteralPath (Join-Path $tlsDir 'ca.crt'))) {
  Invoke-OpenSsl @('req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-sha256', '-days', '3650', '-keyout', (Join-Path $tlsDir 'ca.key'), '-out', (Join-Path $tlsDir 'ca.crt'), '-subj', '/CN=Medusa local device camera CA', '-addext', 'basicConstraints=critical,CA:TRUE', '-addext', 'keyUsage=critical,keyCertSign,cRLSign')
  Invoke-OpenSsl @('x509', '-in', (Join-Path $tlsDir 'ca.crt'), '-outform', 'DER', '-out', (Join-Path $tlsDir 'ca.cer'))
  $regenerate = $true
}
if ($regenerate) {
  $san = 'DNS:localhost,IP:127.0.0.1,' + (($addresses | ForEach-Object { 'IP:' + $_ }) -join ',')
  Set-Content -LiteralPath (Join-Path $tlsDir 'server.ext') -Encoding ascii -Value @("subjectAltName=$san", 'basicConstraints=critical,CA:FALSE', 'keyUsage=critical,digitalSignature,keyEncipherment', 'extendedKeyUsage=serverAuth')
  Invoke-OpenSsl @('req', '-newkey', 'rsa:2048', '-nodes', '-keyout', (Join-Path $tlsDir 'server.key'), '-out', (Join-Path $tlsDir 'server.csr'), '-subj', '/CN=Medusa local device camera')
  Invoke-OpenSsl @('x509', '-req', '-in', (Join-Path $tlsDir 'server.csr'), '-CA', (Join-Path $tlsDir 'ca.crt'), '-CAkey', (Join-Path $tlsDir 'ca.key'), '-CAcreateserial', '-days', '365', '-sha256', '-extfile', (Join-Path $tlsDir 'server.ext'), '-out', (Join-Path $tlsDir 'server.crt'))
  Set-Content -LiteralPath $signaturePath -Value $addressSignature
}
if (-not (Test-Path -LiteralPath (Join-Path $tlsDir 'ca.cer'))) {
  Invoke-OpenSsl @('x509', '-in', (Join-Path $tlsDir 'ca.crt'), '-outform', 'DER', '-out', (Join-Path $tlsDir 'ca.cer'))
}
$pidPath = Join-Path $logDir 'device-camera.pid'
$listener = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
if ($listener) {
  $recordedPid = if (Test-Path -LiteralPath $pidPath) { [int](Get-Content -LiteralPath $pidPath) } else { 0 }
  if ($listener.OwningProcess -ne $recordedPid) { throw "Port $Port is used by another process." }
  if ($regenerate) { Stop-Process -Id $recordedPid -ErrorAction Stop; $listener = $null }
}
if (-not $listener) {
  $env:DEVICE_CAMERA_PORT = [string]$Port
  $nodePath = (Get-Command node -ErrorAction Stop).Source
  $proxyScript = Join-Path $PSScriptRoot 'device-camera-proxy.cjs'
  $process = Start-Process -FilePath $nodePath -ArgumentList ('"' + $proxyScript + '"') -WorkingDirectory $projectDir -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $logDir 'device-camera.out.log') -RedirectStandardError (Join-Path $logDir 'device-camera.err.log')
  Set-Content -LiteralPath $pidPath -Value $process.Id
}
$urls = $addresses | ForEach-Object { "https://${_}:$Port/app/device-scan" }
Set-Content -LiteralPath (Join-Path $logDir 'device-camera-url.txt') -Value $urls
Write-Host 'Phone and computer must be on the same LAN. Install and trust .device-camera-tls/ca.cer on the phone, then open:'
$urls | ForEach-Object { Write-Host $_ }
Write-Host 'Allow camera permission in the browser. No public tunnel is used.'
