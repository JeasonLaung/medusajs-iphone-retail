$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$retailRoot = Join-Path $projectRoot 'retail'
$logRoot = Join-Path $projectRoot 'logs\retail-native'
New-Item -ItemType Directory -Path $logRoot -Force | Out-Null

function Read-Environment([string]$path) {
  if (-not (Test-Path -LiteralPath $path)) { throw "Missing local configuration: $path. See retail/LOCAL-WINDOWS.md." }
  foreach ($line in [IO.File]::ReadAllLines($path)) {
    if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$') {
      $value = $Matches[2].Trim()
      if ($value.Length -ge 2 -and (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'")))) { $value = $value.Substring(1, $value.Length - 2) }
      [Environment]::SetEnvironmentVariable($Matches[1], $value, 'Process')
    }
  }
}

function Test-NodeVersion([string]$exe) {
  $output = & $exe --version
  if ($LASTEXITCODE -ne 0 -or $output -notmatch '^v(\d+)\.(\d+)\.(\d+)$') { return $false }
  return ([int]$Matches[1] -gt 22 -or ([int]$Matches[1] -eq 22 -and ([int]$Matches[2] -gt 23 -or ([int]$Matches[2] -eq 23 -and [int]$Matches[3] -ge 4))))
}

function Stop-NativeWorker([string]$name) {
  # Workers have no TCP listener. Validate the exact executable before stopping
  # the recorded process; never terminate an unrelated reused PID.
  $workerPidPath = Join-Path $logRoot "$name.pid"
  if (Test-Path -LiteralPath $workerPidPath) {
    $workerPIDValue = 0
    if (-not [int]::TryParse(([IO.File]::ReadAllText($workerPidPath)).Trim(), [ref]$workerPIDValue) -or $workerPIDValue -le 4) { throw "Invalid $name PID record." }
    $previousWorker = Get-Process -Id $workerPIDValue -ErrorAction SilentlyContinue
    if ($previousWorker) {
      $expectedExecutable = [IO.Path]::GetFullPath((Join-Path $logRoot "$name.exe"))
      if (-not [string]::Equals($previousWorker.Path, $expectedExecutable, [StringComparison]::OrdinalIgnoreCase)) { throw "Recorded $name PID belongs to another executable." }
      Stop-Process -Id $workerPIDValue -Force -ErrorAction Stop
      $previousWorker.WaitForExit()
    }
    Remove-Item -LiteralPath $workerPidPath
  }
}

function Stop-ProjectPorts {
  Stop-NativeWorker 'order-payment-worker'
  Stop-NativeWorker 'order-refund-worker'
  Stop-NativeWorker 'customer-event-mail-worker'
  # Ask the legacy supervisors to stop first, so they cannot reclaim old ports.
  foreach ($app in @('backend', 'storefront')) {
    & $script:node (Join-Path $PSScriptRoot 'dev-service.cjs') $app stop
    if ($LASTEXITCODE -ne 0) { throw "Cannot stop legacy $app supervisor." }
  }
  Start-Sleep -Seconds 1
  foreach ($port in @(9001,8000,9100,9200,9300)) {
    $owners = @(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess -Unique)
    foreach ($ownerId in $owners) {
      if ($ownerId -le 4 -or $ownerId -eq $PID) { throw "Refusing to stop reserved process on port $port." }
      Write-Host "Closing previous listener on port $port (PID $ownerId)."
      Stop-Process -Id $ownerId -Force -ErrorAction Stop
    }
  }
  for ($attempt=0; $attempt -lt 20; $attempt++) {
    $remaining = @(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Where-Object { $_.LocalPort -in @(9001,8000,9100,9200,9300) })
    if ($remaining.Count -eq 0) { return }
    Start-Sleep -Milliseconds 500
  }
  throw 'Previous project ports are still occupied.'
}

function Start-NativeWorker([string]$name,[string]$command) {
  $workerBinary = Join-Path $logRoot "$name.exe"
  & $script:go -C (Join-Path $retailRoot 'backend') build -o $workerBinary $command
  if ($LASTEXITCODE -ne 0) { throw "$name compilation failed." }
  $worker = Start-Process -FilePath $workerBinary -WorkingDirectory (Join-Path $retailRoot 'backend') -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $logRoot "$name.out.log") -RedirectStandardError (Join-Path $logRoot "$name.err.log")
  Set-Content -LiteralPath (Join-Path $logRoot "$name.pid") -Value $worker.Id
  Start-Sleep -Seconds 2
  $worker.Refresh()
  if ($worker.HasExited) { throw "$name exited. Inspect its native logs." }
  Write-Host "Native $name ready."
}

function Start-LocalApp([string]$name,[string]$exe,[string[]]$arguments,[string]$directory,[string]$url) {
  $launch = @{ FilePath=$exe; WorkingDirectory=$directory; WindowStyle='Hidden'; PassThru=$true; RedirectStandardOutput=(Join-Path $logRoot "$name.out.log"); RedirectStandardError=(Join-Path $logRoot "$name.err.log") }
  if ($arguments.Count -gt 0) { $launch.ArgumentList = $arguments }
  $process = Start-Process @launch
  Set-Content -LiteralPath (Join-Path $logRoot "$name.pid") -Value $process.Id
  for ($attempt=0; $attempt -lt 120; $attempt++) {
    try {
      $response = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 3
      if ($response.StatusCode -eq 200) { Write-Host "$name ready: $url"; return }
    } catch { }
    $process.Refresh()
    if ($process.HasExited) { throw "$name exited. Inspect logs/retail-native/$name.err.log." }
    Start-Sleep -Seconds 1
  }
  throw "$name did not become ready. Inspect logs/retail-native."
}

try {
  $node = if ($env:RETAIL_NODE_EXE) { $env:RETAIL_NODE_EXE } else { (Get-Command node -ErrorAction Stop).Source }
  $bundledNode = Join-Path $env:USERPROFILE '.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe'
  if (-not (Test-NodeVersion $node) -and (Test-Path -LiteralPath $bundledNode)) { $node = $bundledNode }
  if (-not (Test-NodeVersion $node)) { throw 'Node >=22.23.4 required. Set RETAIL_NODE_EXE to a compatible node.exe.' }
  $go = (Get-Command go -ErrorAction Stop).Source
  Read-Environment (Join-Path $retailRoot 'backend\.env')
  if (-not $env:MYSQL_DSN -or -not $env:SESSION_REDIS_URL) { throw 'MYSQL_DSN and SESSION_REDIS_URL are required.' }
  if ($env:APP_PORT -ne '9100' -or $env:APP_BIND -ne '127.0.0.1') { throw 'Local API must bind 127.0.0.1:9100.' }
  foreach ($entry in @('admin\node_modules\vite\bin\vite.js','storefront\node_modules\astro\bin\astro.mjs')) {
    if (-not (Test-Path -LiteralPath (Join-Path $retailRoot $entry))) { throw 'Missing dependencies. Run npm ci in retail/admin and retail/storefront.' }
  }
  Stop-ProjectPorts
  & docker compose -f (Join-Path $retailRoot 'compose.local.yml') up -d mysql
  if ($LASTEXITCODE -ne 0) { throw 'Cannot start project MySQL container.' }
  for ($attempt=0; $attempt -lt 60; $attempt++) {
    $state = & docker inspect --format '{{.State.Health.Status}}' eurorefurb-retail-local-mysql
    if ($state -eq 'healthy') { break }
    if ($attempt -eq 59) { throw 'Project MySQL did not become healthy.' }
    Start-Sleep -Seconds 1
  }
  Write-Host 'Building current Gin API (native apps/local Redis, no automatic database migration)...'
  if ($env:LOCAL_PREVIEW_CATALOG_OPERATIONS_ENABLED -eq 'true') {
    & $go -C (Join-Path $retailRoot 'backend') run ./cmd/configure-local-preview --catalog-operations
    if ($LASTEXITCODE -ne 0) { throw 'Local catalog editing configuration failed.' }
  }
  if ($env:LOCAL_PREVIEW_PRODUCT_ENTRY_ENABLED -eq 'true') {
    & $go -C (Join-Path $retailRoot 'backend') run ./cmd/configure-local-preview --product-entry
    if ($LASTEXITCODE -ne 0) { throw 'Local product entry configuration failed.' }
  }
  if ($env:LOCAL_PREVIEW_CART_ENABLED -eq 'true') {
    & $go -C (Join-Path $retailRoot 'backend') run ./cmd/configure-local-preview --enable
    if ($LASTEXITCODE -ne 0) { throw 'Local draft-cart configuration failed.' }
  }
  if ($env:LOCAL_PREVIEW_PAYMENT_ENABLED -eq 'true') {
    & $go -C (Join-Path $retailRoot 'backend') run ./cmd/configure-local-preview --payments
    if ($LASTEXITCODE -ne 0) { throw 'Local test checkout configuration failed.' }
  }
  if ($env:CHECKOUT_CONTEXT_PAYMENT_ENABLED -eq 'true') {
    & $go -C (Join-Path $retailRoot 'backend') run ./cmd/configure-local-preview --payment-context
    if ($LASTEXITCODE -ne 0) { throw 'Local payment context configuration failed.' }
  }
  if ($env:LOCAL_PREVIEW_FULFILLMENT_ENABLED -eq 'true') {
    & $go -C (Join-Path $retailRoot 'backend') run ./cmd/configure-local-preview --fulfillment
    if ($LASTEXITCODE -ne 0) { throw 'Local fulfillment configuration failed.' }
  }
  if ($env:LOCAL_PREVIEW_BUYBACK_ENABLED -eq 'true') {
    & $go -C (Join-Path $retailRoot 'backend') run ./cmd/configure-local-preview --buyback
    if ($LASTEXITCODE -ne 0) { throw 'Local buyback configuration failed.' }
  }
  if ($env:LOCAL_PREVIEW_AFTER_SALES_ENABLED -eq 'true') {
    & $go -C (Join-Path $retailRoot 'backend') run ./cmd/configure-local-preview --after-sales
    if ($LASTEXITCODE -ne 0) { throw 'Local after-sales configuration failed.' }
  }
  if ($env:LOCAL_PREVIEW_WAREHOUSE_USERS_ENABLED -eq 'true') {
    & $go -C (Join-Path $retailRoot 'backend') run ./cmd/configure-local-preview --warehouse-users
    if ($LASTEXITCODE -ne 0) { throw 'Local warehouse user configuration failed.' }
  }
  if ($env:LOCAL_PREVIEW_WAREHOUSE_MANAGEMENT_ENABLED -eq 'true') {
    & $go -C (Join-Path $retailRoot 'backend') run ./cmd/configure-local-preview --warehouse-management
    if ($LASTEXITCODE -ne 0) { throw 'Local warehouse management configuration failed.' }
  }
  if ($env:LOCAL_PREVIEW_REVIEWS_ENABLED -eq 'true') {
    & $go -C (Join-Path $retailRoot 'backend') run ./cmd/configure-local-preview --reviews
    if ($LASTEXITCODE -ne 0) { throw 'Local product review configuration failed.' }
  }
  if ($env:LOCAL_PREVIEW_ORDER_PAYMENTS_ENABLED -eq 'true') {
    & $go -C (Join-Path $retailRoot 'backend') run ./cmd/configure-local-preview --order-payments
    if ($LASTEXITCODE -ne 0) { throw 'Local supplemental payment configuration failed.' }
  }
  if ($env:LOCAL_PREVIEW_REFUNDS_ENABLED -eq 'true') {
    & $go -C (Join-Path $retailRoot 'backend') run ./cmd/configure-local-preview --refunds
    if ($LASTEXITCODE -ne 0) { throw 'Local refund configuration failed.' }
  }
  if ($env:LOCAL_PREVIEW_NOTIFICATION_MAIL_ENABLED -eq 'true') {
    & $go -C (Join-Path $retailRoot 'backend') run ./cmd/configure-local-preview --notification-mail
    if ($LASTEXITCODE -ne 0) { throw 'Local notification mailbox configuration failed.' }
  }
  $binary = Join-Path $logRoot 'retail-api.exe'
  & $go -C (Join-Path $retailRoot 'backend') build -o $binary ./cmd/api
  if ($LASTEXITCODE -ne 0) { throw 'Gin API compilation failed.' }
  Start-LocalApp 'api' $binary @() (Join-Path $retailRoot 'backend') 'http://127.0.0.1:9100/health'
  if ($env:LOCAL_PREVIEW_ORDER_PAYMENTS_ENABLED -eq 'true') {
    Start-NativeWorker 'order-payment-worker' './cmd/process-order-payments'
  }
  if ($env:LOCAL_PREVIEW_REFUNDS_ENABLED -eq 'true') {
    Start-NativeWorker 'order-refund-worker' './cmd/process-order-refunds'
  }
  if ($env:CUSTOMER_EVENT_MAIL_ENABLED -eq 'true') {
    Start-NativeWorker 'customer-event-mail-worker' './cmd/send-customer-event-mail'
  }
  Start-LocalApp 'admin' $node @('node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','9200','--strictPort') (Join-Path $retailRoot 'admin') 'http://127.0.0.1:9200'
  Read-Environment (Join-Path $retailRoot 'storefront\.env')
  Start-LocalApp 'storefront' $node @('node_modules/astro/bin/astro.mjs','dev','--host','127.0.0.1','--port','9300') (Join-Path $retailRoot 'storefront') 'http://127.0.0.1:9300/dk-en/store'
  Write-Host 'Admin: http://127.0.0.1:9200'
  Write-Host 'Store: http://127.0.0.1:9300/dk-zh/store'
  Write-Host 'Logs: logs/retail-native. MySQL uses Docker 13307; Redis uses native 6379.'
} catch {
  Write-Host $_.Exception.Message -ForegroundColor Red
  exit 1
}
