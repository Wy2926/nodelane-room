# Capture the player's SID BEFORE UAC, including elevation using another admin account.
param([switch]$Rollback, [string]$SourceDir = $PSScriptRoot, [switch]$Quiet, [switch]$ManagedUpdate)
$ErrorActionPreference = 'Stop'
function Invoke-ElevatedInstaller([string]$Installer, [string]$OwnerSid, [string]$SourceDir, [switch]$Rollback, [switch]$Quiet) {
  $pipeName = 'NodeLaneRoom-install-' + [guid]::NewGuid().ToString('N')
  $security = [System.IO.Pipes.PipeSecurity]::new()
  foreach ($sid in @($OwnerSid, 'S-1-5-32-544', 'S-1-5-18')) {
    $security.AddAccessRule([System.IO.Pipes.PipeAccessRule]::new([Security.Principal.SecurityIdentifier]::new($sid), [System.IO.Pipes.PipeAccessRights]::FullControl, [Security.AccessControl.AccessControlType]::Allow))
  }
  $pipe = [System.IO.Pipes.NamedPipeServerStream]::new($pipeName, [System.IO.Pipes.PipeDirection]::In, 1, [System.IO.Pipes.PipeTransmissionMode]::Byte, [System.IO.Pipes.PipeOptions]::Asynchronous, 4096, 4096, $security)
  try {
    $connection = $pipe.BeginWaitForConnection($null, $null)
    $arguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', ('"' + $Installer + '"'), '-OwnerSid', $OwnerSid, '-SourceDir', ('"' + $SourceDir + '"'), '-LogPipe', $pipeName)
    if ($Rollback) { $arguments += '-Rollback' }
    if ($Quiet) { $arguments += '-Quiet' }
    $process = Start-Process -FilePath "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -Verb RunAs -WindowStyle Hidden -PassThru -ArgumentList $arguments
    while (-not $connection.AsyncWaitHandle.WaitOne(100)) {
      if ($process.HasExited) { throw "Elevated installer exited before reporting its status (code $($process.ExitCode))" }
    }
    $pipe.EndWaitForConnection($connection)
    $reader = [IO.StreamReader]::new($pipe, [Text.Encoding]::UTF8)
    try {
      while ($null -ne ($line = $reader.ReadLine())) { Write-Output $line }
    } finally { $reader.Dispose() }
    $process.WaitForExit()
    if ($process.ExitCode -ne 0) { throw "Installer exited with code $($process.ExitCode); see the detailed error above" }
  } finally { $pipe.Dispose() }
}
try {
  $ownerSid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
  $installer = Join-Path $PSScriptRoot 'install.ps1'
  $identity = [System.Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = [System.Security.Principal.WindowsPrincipal]::new($identity)
  if ($ManagedUpdate) {
    if (-not $principal.IsInRole([System.Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Managed updates require the elevated verified updater.' }
    $ownerSid = (Get-Content -LiteralPath (Join-Path $env:ProgramData 'NodeLaneRoom/owner.sid') -Raw).Trim()
    $null = [Security.Principal.SecurityIdentifier]::new($ownerSid)
  }
  if ($principal.IsInRole([System.Security.Principal.WindowsBuiltInRole]::Administrator)) {
    & $installer -OwnerSid $ownerSid -SourceDir $SourceDir -Rollback:$Rollback -Quiet:$Quiet
  } else {
    Invoke-ElevatedInstaller $installer $ownerSid $SourceDir -Rollback:$Rollback -Quiet:$Quiet
  }
} catch {
  Write-Error $_ -ErrorAction Continue
  exit 1
}
