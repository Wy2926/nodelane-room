param([switch]$PurgeState, [switch]$Quiet)
$ErrorActionPreference = 'Stop'
trap {
  Write-Output $_.Exception.Message
  if (-not $Quiet) {
    Add-Type -AssemblyName System.Windows.Forms
    [System.Windows.Forms.MessageBox]::Show($_.Exception.Message, 'NodeLane Room uninstall failed') | Out-Null
  }
  exit 1
}
$identity = [System.Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [System.Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([System.Security.Principal.WindowsBuiltInRole]::Administrator)) {
  $arguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', ('"' + $PSCommandPath + '"'))
  if ($PurgeState) { $arguments += '-PurgeState' }
  if ($Quiet) { $arguments += '-Quiet' }
  $elevated = Start-Process -FilePath "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -Verb RunAs -WindowStyle Hidden -Wait -PassThru -ArgumentList $arguments
  exit $elevated.ExitCode
}
$programFiles = [IO.Path]::GetFullPath($env:ProgramFiles)
$target = [IO.Path]::GetFullPath((Join-Path $programFiles 'NodeLaneRoom'))

function Assert-RemovalTree([string]$Path, [string]$Parent) {
  if ((Split-Path ([IO.Path]::GetFullPath($Path)) -Parent) -ne $Parent) { throw 'Unsafe removal path' }
  if (-not (Test-Path -LiteralPath $Path)) { return }
  foreach ($item in @((Get-Item -LiteralPath $Path)) + @(Get-ChildItem -LiteralPath $Path -Recurse -Force)) {
    if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Refusing reparse point in removal paths' }
    $acl = Get-Acl -LiteralPath $item.FullName
    if ($acl.GetOwner([System.Security.Principal.SecurityIdentifier]).Value -notin @('S-1-5-18', 'S-1-5-32-544')) { throw 'Removal path has an untrusted owner' }
    foreach ($rule in $acl.GetAccessRules($true, $true, [System.Security.Principal.SecurityIdentifier])) {
      if ($rule.AccessControlType -eq 'Allow' -and $rule.IdentityReference.Value -notin @('S-1-5-18', 'S-1-5-32-544', 'S-1-3-0') -and ([int]$rule.FileSystemRights -band 0xD0156) -ne 0) { throw 'Removal path is writable by an untrusted user' }
    }
  }
}

function Remove-Application {
  $bundles = @($target, (Join-Path $programFiles 'NodeLaneRoom.previous'), (Join-Path $programFiles 'NodeLaneRoom.pending'))
  # Validate every tree before stopping services or removing any files.
  foreach ($bundle in $bundles) { Assert-RemovalTree $bundle $programFiles }
  $stateBase = [IO.Path]::GetFullPath($env:ProgramData)
  $state = Join-Path $stateBase 'NodeLaneRoom'
  Assert-RemovalTree $state $stateBase
  $servicePath = Join-Path $target 'nlroom-service.exe'
  $registered = Get-Service -Name NodeLaneRoom -ErrorAction SilentlyContinue
  if ($registered) {
    $config = Get-CimInstance Win32_Service -Filter "Name='NodeLaneRoom'"
    $expected = '"' + $servicePath + '"'
    if (-not $config.PathName.StartsWith($expected, [StringComparison]::OrdinalIgnoreCase) -or $config.StartName -ne 'LocalSystem') { throw 'Unexpected service configuration; uninstall refused' }
  }
  foreach ($process in @(Get-Process -Name nlroom -ErrorAction SilentlyContinue)) {
    if ($process.Path -eq (Join-Path $target 'nlroom.exe')) {
      Stop-Process -Id $process.Id -Force
      if (-not $process.WaitForExit(10000)) { throw 'Client has not exited; installation retained' }
    }
  }
  # Stop first, and wait for the real process before deleting its SCM entry.
  # This also releases TUN, WFP and tunnels when the control server is offline.
  if ($registered -and $registered.Status -ne 'Stopped') {
    Stop-Service -Name NodeLaneRoom -ErrorAction Stop
    $registered.WaitForStatus([System.ServiceProcess.ServiceControllerStatus]::Stopped, [TimeSpan]::FromSeconds(60))
  }
  foreach ($process in @(Get-Process -Name nlroom-service -ErrorAction SilentlyContinue)) {
    if ($process.Path -eq $servicePath -and -not $process.WaitForExit(10000)) { throw 'Networking process has not exited; installation retained' }
  }
  $tapRecord = Join-Path $state 'tap.guid'
  if (Test-Path -LiteralPath $tapRecord) {
    . (Join-Path $PSScriptRoot 'tap.ps1')
    Remove-NodeLaneTap (Join-Path $PSScriptRoot 'tap') ((Get-Content -LiteralPath $tapRecord -Raw).Trim())
    Remove-Item -LiteralPath $tapRecord -Force
  }
  if ($registered) {
    & $servicePath service uninstall
    if ($LASTEXITCODE -ne 0) { throw 'Service removal failed; installation retained' }
  }
  # Remove this install's autostart from the bound player's loaded hive, even
  # when UAC was approved with a different administrator account.
  $ownerFile = Join-Path $state 'owner.sid'
  if (Test-Path -LiteralPath $ownerFile) {
    $ownerSid = (Get-Content -LiteralPath $ownerFile -Raw).Trim()
    $null = [System.Security.Principal.SecurityIdentifier]::new($ownerSid)
    $userVersion = 'Registry::HKEY_USERS\' + $ownerSid + '\Software\Microsoft\Windows\CurrentVersion'
    $runPath = $userVersion + '\Run'
    if (Test-Path -LiteralPath $runPath) {
      $runKey = Get-Item -LiteralPath $runPath
      foreach ($name in $runKey.GetValueNames()) {
        $value = $runKey.GetValue($name)
        if ($value -in @((Join-Path $target 'nlroom.exe'), ('"' + (Join-Path $target 'nlroom.exe') + '"'))) {
          Remove-ItemProperty -LiteralPath $runPath -Name $name
          Remove-ItemProperty -LiteralPath ($userVersion + '\Explorer\StartupApproved\Run') -Name $name -ErrorAction SilentlyContinue
        }
      }
    }
  }
  # Keep the current uninstaller available until optional data and backups are removed.
  if ($PurgeState -and (Test-Path -LiteralPath $state)) { Remove-Item -LiteralPath $state -Recurse -Force }
  foreach ($bundle in @($bundles[1], $bundles[2], $target)) {
    Assert-RemovalTree $bundle $programFiles
    if (Test-Path -LiteralPath $bundle) { Remove-Item -LiteralPath $bundle -Recurse -Force }
  }
  $shortcut = Join-Path ([Environment]::GetFolderPath('CommonPrograms')) 'NodeLane Room.lnk'
  if (Test-Path -LiteralPath $shortcut) { Remove-Item -LiteralPath $shortcut -Force }
  $registry = 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\NodeLaneRoom'
  if (Test-Path -LiteralPath $registry) { Remove-Item -LiteralPath $registry -Force }
}

$lock = [IO.File]::Open((Join-Path $programFiles 'NodeLaneRoom.install.lock'), 'OpenOrCreate', 'ReadWrite', 'None')
try {
  Remove-Application
  Write-Output 'Uninstalled. Identity is preserved unless -PurgeState was supplied.'
} finally { $lock.Dispose() }
