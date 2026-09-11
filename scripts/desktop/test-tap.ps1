# All native commands and device/registry queries are mocked; no adapter is changed.
$ErrorActionPreference = 'Stop'
$root = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$ownedID = '{a986c01f-4256-4921-bce7-d3d3fb61898b}'
$foreignID = '{9ea44b15-df8e-4211-b5e5-80543d159a94}'
foreach ($scenario in @('create', 'existing', 'legacy-id', 'foreign-name', 'old-driver', 'registry-denied', 'stage-failure', 'stage-restart', 'create-failure', 'ready-failure', 'ready-registry-denied', 'delete-owned', 'delete-foreign', 'delete-missing')) {
  & {
    . (Join-Path $PSScriptRoot 'tap.ps1')
    $directory = Join-Path $root '.local/mock-tap'
    $script:stageCalls = 0; $script:createCalls = 0; $script:deleted = $null
    $script:adapters = @([pscustomobject]@{ Name = 'Other VPN'; InterfaceGuid = $foreignID })
    if ($scenario -in @('existing', 'legacy-id', 'foreign-name', 'old-driver', 'registry-denied', 'delete-owned')) {
      $script:adapters += [pscustomobject]@{ Name = 'nodelane0-lan'; InterfaceGuid = $ownedID }
    }
    function Assert-TapPackage { param($Directory) }
    function Get-NetAdapter { param([switch]$IncludeHidden, $ErrorAction) $script:adapters }
    $class = 'HKLM:\SYSTEM\CurrentControlSet\Control\Class\{4D36E972-E325-11CE-BFC1-08002BE10318}'
    function Get-ChildItem {
      param($LiteralPath, [switch]$Name, $ErrorAction)
      if ($LiteralPath -ne $class) { throw 'Unexpected registry enumeration' }
      # Windows exposes protected non-driver keys here, including Properties.
      if (-not $Name) { throw [Security.SecurityException]::new('Requested registry access is not allowed: Properties') }
      '0001'; 'Properties'; 'Configuration'
    }
    function Get-ItemProperty {
      param($LiteralPath, $ErrorAction)
      if ($LiteralPath -ne (Join-Path $class '0001')) { throw 'Opened a non-driver registry key' }
      if ($scenario -in @('registry-denied', 'ready-registry-denied')) { Write-Error 'Driver registry access denied' -ErrorAction $ErrorAction; return }
      [pscustomobject]@{
        NetCfgInstanceId = $ownedID
        ComponentId = if ($scenario -in @('foreign-name', 'ready-failure')) { 'other' } elseif ($scenario -eq 'legacy-id') { 'tap0901' } else { 'root\tap0901' }
        DriverVersion = if ($scenario -eq 'old-driver') { '9.24.7.0' } else { '9.27.0.0' }
      }
    }
    function Start-Sleep { param($Milliseconds) }
    $pnp = "$env:SystemRoot\System32\pnputil.exe"
    $tapctl = Join-Path $directory 'tapctl.exe'
    Set-Item -Path ('Function:' + $pnp) -Value {
      if ($args.Count -ne 2 -or $args[0] -ne '/add-driver' -or $args[1] -ne (Join-Path $directory 'OemVista.inf')) { throw 'Attempted to update unrelated TAP adapters' }
      $script:stageCalls++
      $global:LASTEXITCODE = if ($scenario -eq 'stage-failure') { 1 } elseif ($scenario -eq 'stage-restart') { 3010 } else { 0 }
    }
    Set-Item -Path ('Function:' + $tapctl) -Value {
      $global:LASTEXITCODE = 0
      if ($args[0] -eq 'create') {
        if (($args -join ' ') -ne 'create --name nodelane0-lan --hwid root\tap0901') { throw 'Wrong adapter creation request' }
        $script:createCalls++
        if ($scenario -eq 'create-failure') { $global:LASTEXITCODE = 1; return }
        $script:adapters += [pscustomobject]@{ Name = 'nodelane0-lan'; InterfaceGuid = $ownedID }
        return $ownedID
      }
      if ($args.Count -ne 2 -or $args[0] -ne 'delete' -or $args[1] -ne $ownedID) { throw 'Attempted to delete an unrelated adapter' }
      $script:deleted = $args[1]
    }
    try {
      foreach ($command in @($pnp, $tapctl)) {
        if ((Get-Command $command).CommandType -ne 'Function') { throw 'Native command mock is missing' }
      }
      $failure = $null; $result = $null
      try {
        if ($scenario.StartsWith('delete-')) {
          $id = if ($scenario -eq 'delete-foreign') { $foreignID } else { $ownedID }
          Remove-NodeLaneTap $directory $id
        } else { $result = Install-NodeLaneTap $directory }
      } catch { $failure = $_.Exception.Message }
      $expectedFailure = $scenario -in @('foreign-name', 'old-driver', 'registry-denied', 'stage-failure', 'stage-restart', 'create-failure', 'ready-failure', 'ready-registry-denied', 'delete-foreign')
      if ([bool]$failure -ne $expectedFailure) { throw "Unexpected TAP result: $scenario ($failure)" }
      if ($scenario -eq 'create' -and $result -ne $ownedID) { throw 'Created adapter ownership was not returned' }
      if ($scenario -in @('existing', 'legacy-id') -and $null -ne $result) { throw 'Claimed ownership of a pre-existing adapter' }
      if ($scenario -in @('registry-denied', 'ready-registry-denied') -and $failure -ne 'Driver registry access denied') { throw 'Suppressed driver registry verification error' }
      if ($scenario -in @('existing', 'legacy-id', 'foreign-name', 'old-driver', 'registry-denied', 'delete-owned', 'delete-foreign', 'delete-missing') -and ($script:stageCalls -or $script:createCalls)) { throw 'Unexpected adapter installation' }
      if ($scenario -in @('stage-failure', 'stage-restart') -and $script:createCalls) { throw 'Created adapter after staging failure' }
      $expectedDelete = $scenario -in @('ready-failure', 'ready-registry-denied', 'delete-owned')
      if ([bool]$script:deleted -ne $expectedDelete) { throw 'Wrong adapter cleanup result' }
      Write-Output "PASS TAP management: $scenario"
    } finally {
      Remove-Item -LiteralPath ('Function:' + $pnp), ('Function:' + $tapctl)
    }
  }
}
