# Capture the player's SID BEFORE UAC, including elevation using another admin account.
$ErrorActionPreference = 'Stop'
try {
  $ownerSid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
  $installer = Join-Path $PSScriptRoot 'install.ps1'
  $identity = [System.Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = [System.Security.Principal.WindowsPrincipal]::new($identity)
  if ($principal.IsInRole([System.Security.Principal.WindowsBuiltInRole]::Administrator)) {
    & $installer -OwnerSid $ownerSid
  } else {
    $process = Start-Process -FilePath "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -Verb RunAs -Wait -PassThru -ArgumentList @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', ('"' + $installer + '"'), '-OwnerSid', $ownerSid)
    if ($process.ExitCode -ne 0) { throw "Installer exited with code $($process.ExitCode)" }
  }
} catch {
  Write-Error $_
  exit 1
}
