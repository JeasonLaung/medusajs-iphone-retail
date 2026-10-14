$ErrorActionPreference = 'Stop'
$version = '0.71.0'
$projectDir = Split-Path -Parent $PSScriptRoot
$binDir = Join-Path $projectDir 'frp\bin'
$archiveName = "frp_${version}_windows_amd64.zip"
$baseUrl = "https://github.com/fatedier/frp/releases/download/v$version"
$tempDir = Join-Path ([IO.Path]::GetTempPath()) ("medusa-frp-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
try {
  $archive = Join-Path $tempDir $archiveName
  Invoke-WebRequest "$baseUrl/$archiveName" -OutFile $archive -UseBasicParsing
  $checksums = (Invoke-WebRequest "$baseUrl/frp_sha256_checksums.txt" -UseBasicParsing).Content
  if ($checksums -is [byte[]]) { $checksums = [Text.Encoding]::UTF8.GetString($checksums) }
  $checksumLine = $checksums -split "`n" | Where-Object { $_.Trim().EndsWith($archiveName) } | Select-Object -First 1
  if (-not $checksumLine) { throw 'Official checksum was not found.' }
  $expected = ($checksumLine.Trim() -split '\s+')[0]
  $actual = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash
  if ($actual -ne $expected) { throw 'frp download checksum mismatch.' }
  Expand-Archive -LiteralPath $archive -DestinationPath $tempDir
  New-Item -ItemType Directory -Path $binDir -Force | Out-Null
  Copy-Item -LiteralPath (Join-Path $tempDir "frp_${version}_windows_amd64\frpc.exe") -Destination $binDir -Force
  Copy-Item -LiteralPath (Join-Path $tempDir "frp_${version}_windows_amd64\LICENSE") -Destination $binDir -Force
  Write-Host "Installed frpc $version. Official SHA256 verified."
} finally {
  # Only remove the specific temporary directory created by this script.
  $resolvedTemp = [IO.Path]::GetFullPath($tempDir)
  $tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
  if ($resolvedTemp.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -and
      (Split-Path -Leaf $resolvedTemp) -match '^medusa-frp-[0-9a-f]{32}$') {
    Remove-Item -LiteralPath $resolvedTemp -Recurse -Force
  }
}
