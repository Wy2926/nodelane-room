#Requires -RunAsAdministrator
param([switch]$PurgeState)
$ErrorActionPreference = 'Stop'
$programFiles = [IO.Path]::GetFullPath($env:ProgramFiles)
$target = [IO.Path]::GetFullPath((Join-Path $programFiles 'NodeLaneRoom'))
if ((Split-Path $target -Parent) -ne $programFiles) { throw 'Unsafe installation path' }
if ((Get-Item -LiteralPath $target).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Refusing reparse point' }
$cli = Join-Path $target 'nlroom-cli.exe'
$service = Join-Path $target 'nlroom-service.exe'
# Leaving is best effort when the control server is unreachable. The service
# stop still releases local TUN, WFP sessions, tunnels and discovery proxies.
try { & $cli room leave 2>$null | Out-Null } catch { Write-Verbose 'Control unavailable; stopping local service still removes local network resources.' }
if (Get-Service -Name NodeLaneRoom -ErrorAction SilentlyContinue) {
  & $service service uninstall
  if ($LASTEXITCODE -ne 0) { throw 'Service removal failed; installation retained' }
}
Remove-Item -LiteralPath $target -Recurse -Force
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
