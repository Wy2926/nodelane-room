param([switch]$PurgeState)
$ErrorActionPreference = 'Stop'
$identity = [System.Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [System.Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([System.Security.Principal.WindowsBuiltInRole]::Administrator)) {
  $arguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', ('"' + $PSCommandPath + '"'))
  if ($PurgeState) { $arguments += '-PurgeState' }
  $elevated = Start-Process -FilePath "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -Verb RunAs -WindowStyle Hidden -Wait -PassThru -ArgumentList $arguments
  exit $elevated.ExitCode
}
$programFiles = [IO.Path]::GetFullPath($env:ProgramFiles)
$target = [IO.Path]::GetFullPath((Join-Path $programFiles 'NodeLaneRoom'))
if ((Split-Path $target -Parent) -ne $programFiles) { throw 'Unsafe installation path' }
if ((Get-Item -LiteralPath $target).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Refusing reparse point' }
$cli = Join-Path $target 'nlroom-cli.exe'
$service = Join-Path $target 'nlroom-service.exe'
$guiPath = Join-Path $target 'nlroom.exe'
foreach ($process in (Get-Process -Name nlroom -ErrorAction SilentlyContinue)) {
  if ($process.Path -eq $guiPath) { Stop-Process -Id $process.Id -Force }
}
# Leaving is best effort when the control server is unreachable. The service
# stop still releases local TUN, WFP sessions and tunnels.
try { & $cli room leave 2>$null | Out-Null } catch { Write-Verbose 'Control unavailable; stopping local service still removes local network resources.' }
if (Get-Service -Name NodeLaneRoom -ErrorAction SilentlyContinue) {
  & $service service uninstall
  if ($LASTEXITCODE -ne 0) { throw 'Service removal failed; installation retained' }
}
Remove-Item -LiteralPath $target -Recurse -Force
$shortcut = Join-Path ([Environment]::GetFolderPath('CommonPrograms')) 'NodeLane Room.lnk'
if (Test-Path -LiteralPath $shortcut) { Remove-Item -LiteralPath $shortcut -Force }
$registry = 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\NodeLaneRoom'
if (Test-Path -LiteralPath $registry) { Remove-Item -LiteralPath $registry -Force }
if ($PurgeState) {
  $base = [IO.Path]::GetFullPath($env:ProgramData)
  $state = [IO.Path]::GetFullPath((Join-Path $base 'NodeLaneRoom'))
  if ((Split-Path $state -Parent) -ne $base) { throw 'Unsafe state path' }
  if (Test-Path -LiteralPath $state) {
    if ((Get-Item -LiteralPath $state).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Refusing reparse point' }
    Remove-Item -LiteralPath $state -Recurse -Force
  }
}
Write-Output 'Uninstalled. Identity is preserved unless -PurgeState was supplied.'
