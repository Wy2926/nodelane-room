# Capture the player's SID BEFORE UAC, including elevation using another admin account.
param([switch]$Rollback)
$ErrorActionPreference = 'Stop'
try {
  $ownerSid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
  $installer = Join-Path $PSScriptRoot 'install.ps1'
  $identity = [System.Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = [System.Security.Principal.WindowsPrincipal]::new($identity)
  if ($principal.IsInRole([System.Security.Principal.WindowsBuiltInRole]::Administrator)) {
    & $installer -OwnerSid $ownerSid -Rollback:$Rollback
  } else {
    $arguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', ('"' + $installer + '"'), '-OwnerSid', $ownerSid)
    if ($Rollback) { $arguments += '-Rollback' }
    $process = Start-Process -FilePath "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -Verb RunAs -WindowStyle Hidden -Wait -PassThru -ArgumentList $arguments
    if ($process.ExitCode -ne 0) { throw "Installer exited with code $($process.ExitCode)" }
  }
} catch {
  Write-Error $_
  exit 1
}
