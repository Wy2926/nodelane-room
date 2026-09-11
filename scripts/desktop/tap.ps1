# Only manage NodeLane's dedicated adapter; never update or remove other VPN devices.
function Assert-TapPackage([string]$Directory) {
  $expected = @('OemVista.inf', 'tap0901.cat', 'tap0901.sys', 'tapctl.exe')
  $seen = @{}
  foreach ($line in (Get-Content -LiteralPath (Join-Path $Directory 'SHA256SUMS'))) {
    if ($line -notmatch '^([0-9a-f]{64})  ([A-Za-z0-9.]+)$') { throw 'Invalid TAP package manifest' }
    $hash = $Matches[1]; $name = $Matches[2]
    if ($name -notin $expected -or $seen.ContainsKey($name)) { throw 'Unexpected TAP package file' }
    $file = Join-Path $Directory $name
    if ((Get-Item -LiteralPath $file).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Refusing TAP package reparse point' }
    if ((Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant() -ne $hash) { throw "TAP package checksum mismatch: $name" }
    $seen[$name] = $true
  }
  if ($seen.Count -ne $expected.Count) { throw 'Incomplete TAP package' }
  foreach ($name in @('tap0901.cat', 'tap0901.sys', 'tapctl.exe')) {
    $signature = Get-AuthenticodeSignature -LiteralPath (Join-Path $Directory $name)
    $publisher = if ($name -eq 'tapctl.exe') { 'OpenVPN Inc\.' } else { 'Microsoft Corporation' }
    if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch ('(?:^|, )O=' + $publisher + '(?:,|$)')) { throw "TAP signature verification failed: $name" }
  }
}

function Get-NodeLaneTap {
  $matches = @(Get-NetAdapter -IncludeHidden -ErrorAction Stop | Where-Object Name -eq 'nodelane0-lan')
  if ($matches.Count -gt 1) { throw 'Multiple NodeLane TAP adapters found' }
  if ($matches.Count -eq 0) { return $null }
  $adapter = $matches[0]
  $guid = ([guid]$adapter.InterfaceGuid).ToString('B')
  $class = 'HKLM:\SYSTEM\CurrentControlSet\Control\Class\{4D36E972-E325-11CE-BFC1-08002BE10318}'
  # Enumerate names first: opening every child also touches protected Properties.
  $driver = @(Get-ChildItem -LiteralPath $class -Name -ErrorAction Stop |
    Where-Object { $_ -match '^\d{4}$' } |
    ForEach-Object { Get-ItemProperty -LiteralPath (Join-Path $class $_) -ErrorAction Stop } |
    Where-Object { $_.NetCfgInstanceId -eq $guid -and $_.ComponentId -in @('tap0901', 'root\tap0901') })
  if ($driver.Count -ne 1) { throw 'nodelane0-lan belongs to another driver; it will not be changed' }
  if ([version]$driver[0].DriverVersion -lt [version]'9.27.0.0') { throw 'The dedicated TAP adapter needs driver 9.27.0 or newer; remove only nodelane0-lan and retry' }
  return $adapter
}

function Remove-NodeLaneTap([string]$Directory, [string]$Guid) {
  $id = ([guid]$Guid).ToString('B')
  Assert-TapPackage $Directory
  $adapter = @(Get-NetAdapter -IncludeHidden -ErrorAction Stop | Where-Object { ([guid]$_.InterfaceGuid).ToString('B') -eq $id })
  if ($adapter.Count -eq 0) { return }
  $owned = Get-NodeLaneTap
  if (-not $owned -or ([guid]$owned.InterfaceGuid).ToString('B') -ne $id) { throw 'Recorded TAP adapter no longer matches NodeLane; removal refused' }
  & (Join-Path $Directory 'tapctl.exe') delete $id | Out-Null
  if ($LASTEXITCODE -ne 0) { throw 'Could not remove the dedicated TAP adapter; retry after restarting Windows' }
}

function Install-NodeLaneTap([string]$Directory) {
  Assert-TapPackage $Directory
  if (Get-NodeLaneTap) { return $null }
  # Stage only. /install would also update unrelated adapters sharing tap0901.
  & "$env:SystemRoot\System32\pnputil.exe" /add-driver (Join-Path $Directory 'OemVista.inf') | Out-Null
  if ($LASTEXITCODE -eq 3010) { throw 'TAP driver staging requires a Windows restart; restart and retry' }
  if ($LASTEXITCODE -ne 0) { throw 'TAP driver staging failed; inspect Windows setupapi.dev.log' }
  $output = & (Join-Path $Directory 'tapctl.exe') create --name nodelane0-lan --hwid 'root\tap0901'
  if ($LASTEXITCODE -ne 0) { throw 'Could not create the dedicated TAP adapter' }
  $id = ([guid]($output -join '').Trim()).ToString('B')
  try {
    for ($attempt = 0; $attempt -lt 50; $attempt++) {
      $adapter = Get-NodeLaneTap
      if ($adapter -and ([guid]$adapter.InterfaceGuid).ToString('B') -eq $id) { return $id }
      Start-Sleep -Milliseconds 200
    }
    throw 'TAP adapter did not become available; restart Windows and retry'
  } catch {
    # Only remove a GUID returned by this successful creation, never discovery.
    & (Join-Path $Directory 'tapctl.exe') delete $id | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'TAP setup failed and the new adapter needs manual removal' }
    throw
  }
}
