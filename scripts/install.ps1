#Requires -RunAsAdministrator
param([string]$OwnerSid = ([System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value))
$ErrorActionPreference = 'Stop'
$null = [System.Security.Principal.SecurityIdentifier]::new($OwnerSid)
$build = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'BUILD.txt') -Raw
if ($build -notmatch 'Target: windows/(amd64|arm64)') { throw 'Missing Windows package architecture' }
$arch = $Matches[1]
$nativeArch = $env:PROCESSOR_ARCHITECTURE
if ($env:PROCESSOR_ARCHITEW6432) { $nativeArch = $env:PROCESSOR_ARCHITEW6432 }
if ($nativeArch.ToLowerInvariant() -ne $arch) { throw "Use the $nativeArch package for this Windows installation" }
$driver = "dist/windows/wintun/bin/$arch/wintun.dll"
$files = @('nodelane.exe', 'install.ps1', 'uninstall.ps1', 'NodeLane.cmd', 'BUILD.txt', 'THIRD_PARTY_NOTICES.txt', $driver, 'dist/windows/wintun/LICENSE.txt')
foreach ($file in $files) {
  if (-not (Test-Path -LiteralPath (Join-Path $PSScriptRoot $file) -PathType Leaf)) { throw "Incomplete package: $file" }
}
if (-not (Test-Path -LiteralPath (Join-Path $PSScriptRoot 'licenses') -PathType Container)) { throw 'Incomplete package: licenses' }
$signature = Get-AuthenticodeSignature -LiteralPath (Join-Path $PSScriptRoot $driver)
if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch 'O=WireGuard LLC(?:,|$)') { throw 'Wintun signature verification failed' }
$target = Join-Path $env:ProgramFiles 'NodeLaneRoom'
if (Get-Service -Name NodeLaneRoom -ErrorAction SilentlyContinue) { throw 'Uninstall the existing service before upgrading. Identity is preserved.' }
if (Test-Path -LiteralPath $target) {
  $item = Get-Item -LiteralPath $target
  if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Installation directory must not be a reparse point' }
}
New-Item -ItemType Directory -Force -Path $target | Out-Null
$acl = New-Object System.Security.AccessControl.DirectorySecurity
$acl.SetOwner([System.Security.Principal.SecurityIdentifier]::new('S-1-5-32-544'))
$acl.SetAccessRuleProtection($true, $false)
foreach ($entry in @(@('S-1-5-18','FullControl'), @('S-1-5-32-544','FullControl'), @('S-1-5-32-545','ReadAndExecute'))) {
  $sid = [System.Security.Principal.SecurityIdentifier]::new($entry[0])
  $rule = [System.Security.AccessControl.FileSystemAccessRule]::new($sid, [System.Security.AccessControl.FileSystemRights]$entry[1], [System.Security.AccessControl.InheritanceFlags]'ContainerInherit,ObjectInherit', [System.Security.AccessControl.PropagationFlags]::None, [System.Security.AccessControl.AccessControlType]::Allow)
  $acl.AddAccessRule($rule)
}
Set-Acl -LiteralPath $target -AclObject $acl
# Copy only the release payload, never arbitrary files added beside the installer.
foreach ($file in $files) {
  $destination = Join-Path $target $file
  New-Item -ItemType Directory -Force -Path (Split-Path $destination -Parent) | Out-Null
  Copy-Item -LiteralPath (Join-Path $PSScriptRoot $file) -Destination $destination -Force
}
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'licenses') -Destination $target -Recurse -Force
& (Join-Path $target 'nodelane.exe') service install --owner-sid $OwnerSid
if ($LASTEXITCODE -ne 0) { throw 'Service registration failed; inspect the error before retrying' }
$service = Get-Service -Name NodeLaneRoom
$service.WaitForStatus([System.ServiceProcess.ServiceControllerStatus]::Running, [TimeSpan]::FromSeconds(20))
$ready = $false
for ($attempt = 0; $attempt -lt 40; $attempt++) {
  try {
    & (Join-Path $target 'nodelane.exe') status 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) { $ready = $true; break }
  } catch { Write-Verbose 'Waiting for the local control pipe.' }
  Start-Sleep -Milliseconds 250
}
if (-not $ready) { throw 'Service started but the local control pipe is not ready; inspect the service log' }
Write-Output "Installed. Use: & '$target/nodelane.exe' init --server https://room.example.com --name Player"
