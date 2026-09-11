#Requires -RunAsAdministrator
param([string]$OwnerSid = ([System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value), [switch]$Rollback, [switch]$CheckOnly, [string]$SourceDir = $PSScriptRoot, [switch]$Quiet, [string]$LogPipe)
$ErrorActionPreference = 'Stop'
function Write-InstallMessage([string]$Message) {
  Write-Output $Message
  if ($script:installLog) { $script:installLog.WriteLine($Message) }
}
trap {
  Write-InstallMessage ('Installation failed: ' + $_.Exception.Message)
  if (-not $CheckOnly -and -not $Quiet) {
    Add-Type -AssemblyName System.Windows.Forms
    [System.Windows.Forms.MessageBox]::Show($_.Exception.Message, 'NodeLane Room installation failed') | Out-Null
  }
  exit 1
}
if ($LogPipe) {
  if ($LogPipe -notmatch '^NodeLaneRoom-install-[0-9a-f]{32}$') { throw 'Invalid installation status pipe' }
  $pipe = [IO.Pipes.NamedPipeClientStream]::new('.', $LogPipe, [IO.Pipes.PipeDirection]::Out)
  $pipe.Connect(10000)
  $script:installLog = [IO.StreamWriter]::new($pipe, [Text.UTF8Encoding]::new($false))
  $script:installLog.AutoFlush = $true
}
Write-InstallMessage 'Checking package, installation permissions and existing service...'
$null = [System.Security.Principal.SecurityIdentifier]::new($OwnerSid)
if (-not [Environment]::Is64BitProcess) { throw 'Run the installer with native 64-bit PowerShell' }
$base = [IO.Path]::GetFullPath($env:ProgramFiles)
$target = Join-Path $base 'NodeLaneRoom'
$previous = Join-Path $base 'NodeLaneRoom.previous'
$pending = Join-Path $base 'NodeLaneRoom.pending'
$source = [IO.Path]::GetFullPath($SourceDir)
if ($Rollback) { $source = $previous }

function Assert-Tree([string]$Path, [switch]$Trusted) {
  if (-not (Test-Path -LiteralPath $Path)) { return }
  $items = @((Get-Item -LiteralPath $Path)) + @(Get-ChildItem -LiteralPath $Path -Recurse -Force)
  foreach ($item in $items) {
    if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Refusing reparse points in installation paths' }
    if ($Trusted) {
      $itemAcl = Get-Acl -LiteralPath $item.FullName
      $owner = $itemAcl.GetOwner([System.Security.Principal.SecurityIdentifier]).Value
      if ($owner -notin @('S-1-5-18', 'S-1-5-32-544')) { throw 'Installation path has an untrusted owner' }
      foreach ($rule in $itemAcl.GetAccessRules($true, $true, [System.Security.Principal.SecurityIdentifier])) {
        if ($rule.AccessControlType -eq 'Allow' -and $rule.IdentityReference.Value -notin @('S-1-5-18', 'S-1-5-32-544', 'S-1-3-0') -and ([int]$rule.FileSystemRights -band 0xD0156) -ne 0) { throw 'Installation path is writable by an untrusted user' }
      }
    }
  }
}
function Assert-Bundle([string]$Path) {
  $resolved = [IO.Path]::GetFullPath($Path)
  if ((Split-Path $resolved -Parent) -ne $base -or $resolved -notin @($target, $previous, $pending)) { throw 'Unsafe installation path' }
  Assert-Tree $resolved -Trusted
}
function Remove-Bundle([string]$Path) {
  Assert-Bundle $Path
  if (Test-Path -LiteralPath $Path) { Remove-Item -LiteralPath $Path -Recurse -Force }
}
function Move-Bundle([string]$From, [string]$To) {
  Assert-Bundle $From
  Assert-Bundle $To
  Move-Item -LiteralPath $From -Destination $To
}
function Protect-Bundle([string]$Path) {
  $acl = [System.Security.AccessControl.DirectorySecurity]::new()
  $acl.SetOwner([System.Security.Principal.SecurityIdentifier]::new('S-1-5-32-544'))
  $acl.SetAccessRuleProtection($true, $false)
  foreach ($entry in @(@('S-1-5-18','FullControl'), @('S-1-5-32-544','FullControl'), @('S-1-5-32-545','ReadAndExecute'))) {
    $acl.AddAccessRule([System.Security.AccessControl.FileSystemAccessRule]::new([System.Security.Principal.SecurityIdentifier]::new($entry[0]), [System.Security.AccessControl.FileSystemRights]$entry[1], [System.Security.AccessControl.InheritanceFlags]'ContainerInherit,ObjectInherit', [System.Security.AccessControl.PropagationFlags]::None, [System.Security.AccessControl.AccessControlType]::Allow))
  }
  Set-Acl -LiteralPath $Path -AclObject $acl
}
function Stop-Network {
  $service = Get-Service -Name NodeLaneRoom -ErrorAction SilentlyContinue
  if ($service -and $service.Status -ne 'Stopped') {
    Stop-Service -Name NodeLaneRoom -ErrorAction Stop
    $service.WaitForStatus([System.ServiceProcess.ServiceControllerStatus]::Stopped, [TimeSpan]::FromSeconds(60))
  }
  foreach ($process in @(Get-Process -Name nlroom-service -ErrorAction SilentlyContinue)) {
    if ($process.Path -eq (Join-Path $target 'nlroom-service.exe') -and -not $process.WaitForExit(10000)) { throw 'Networking process has not exited' }
  }
}
function Wait-Ready([string]$Version) {
  $service = Get-Service -Name NodeLaneRoom
  $service.WaitForStatus([System.ServiceProcess.ServiceControllerStatus]::Running, [TimeSpan]::FromSeconds(30))
  for ($attempt = 0; $attempt -lt 30; $attempt++) {
    try {
      $json = & (Join-Path $target 'nlroom-cli.exe') status --json 2>$null
      if ($LASTEXITCODE -eq 0) {
        $status = $json | ConvertFrom-Json
        if ($status.version -eq $Version -and $status.protocol_version -eq 2) { return }
      }
    } catch { Write-Verbose 'Waiting for the local service.' }
    Start-Sleep -Milliseconds 300
  }
  throw 'The installed service did not become ready with the expected version'
}

function Register-Application([string]$Version) {
  if (Test-Path -LiteralPath (Join-Path $target 'nlroom.exe')) {
    $managedGUI = Test-Path -LiteralPath (Join-Path $target 'Uninstall.exe')
    $registry = 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\NodeLaneRoom'
    New-Item -Path $registry -Force | Out-Null
    $properties = @{
      DisplayName = 'NodeLane Room'; DisplayVersion = $version; Publisher = 'NodeLane'
      InstallLocation = $target; DisplayIcon = (Join-Path $target 'nlroom.exe')
      UninstallString = if ($managedGUI) { '"' + (Join-Path $target 'Uninstall.exe') + '"' } else { '"' + "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" + '" -NoProfile -ExecutionPolicy Bypass -File "' + (Join-Path $target 'uninstall.ps1') + '"' }
    }
    foreach ($entry in $properties.GetEnumerator()) { New-ItemProperty -Path $registry -Name $entry.Key -Value $entry.Value -PropertyType String -Force | Out-Null }
    if ($managedGUI) {
      New-ItemProperty -Path $registry -Name QuietUninstallString -Value ('"' + (Join-Path $target 'Uninstall.exe') + '" /S') -PropertyType String -Force | Out-Null
    } else {
      Remove-ItemProperty -Path $registry -Name QuietUninstallString -ErrorAction SilentlyContinue
    }
    foreach ($name in @('NoModify', 'NoRepair')) { New-ItemProperty -Path $registry -Name $name -Value 1 -PropertyType DWord -Force | Out-Null }
    $size = [int][Math]::Ceiling((Get-ChildItem -LiteralPath $target -Recurse -File | Measure-Object -Property Length -Sum).Sum / 1KB)
    New-ItemProperty -Path $registry -Name EstimatedSize -Value $size -PropertyType DWord -Force | Out-Null
    $shortcut = (New-Object -ComObject WScript.Shell).CreateShortcut((Join-Path ([Environment]::GetFolderPath('CommonPrograms')) 'NodeLane Room.lnk'))
    $shortcut.TargetPath = Join-Path $target 'nlroom.exe'; $shortcut.WorkingDirectory = $target; $shortcut.Save()
  }
}

function Unregister-Application {
  $registry = 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\NodeLaneRoom'
  if (Test-Path -LiteralPath $registry) { Remove-Item -LiteralPath $registry -Force }
  $shortcut = Join-Path ([Environment]::GetFolderPath('CommonPrograms')) 'NodeLane Room.lnk'
  if (Test-Path -LiteralPath $shortcut) { Remove-Item -LiteralPath $shortcut -Force }
}

Assert-Tree $source
foreach ($path in @($target, $previous, $pending)) { Assert-Bundle $path }
$build = Get-Content -LiteralPath (Join-Path $source 'BUILD.txt') -Raw
if ($build -notmatch '(?m)^Target: windows/(amd64|arm64)\s*$') { throw 'Missing Windows package architecture' }
$arch = $Matches[1]
if ($build -notmatch '(?m)^Version: (\d+\.\d+\.\d+)\s*$') { throw 'Invalid package version' }
$version = $Matches[1]
$nativeArch = switch (@(Get-CimInstance Win32_Processor -Property Architecture)[0].Architecture) { 9 { 'amd64' }; 12 { 'arm64' }; default { throw 'Unsupported native Windows architecture' } }
if ($nativeArch -ne $arch) { throw "Use the $nativeArch package for this Windows installation" }
$files = @('nlroom-cli.exe', 'nlroom-service.exe', 'BUILD.txt', 'THIRD_PARTY_NOTICES.txt')
$hasGUI = Test-Path -LiteralPath (Join-Path $source 'nlroom.exe') -PathType Leaf
$managedGUI = $hasGUI -and (Test-Path -LiteralPath (Join-Path $source 'Uninstall.exe') -PathType Leaf)
if (-not $hasGUI -and (Test-Path -LiteralPath (Join-Path $target 'nlroom.exe'))) { throw 'Use the complete desktop installer to update this GUI installation' }
if ($managedGUI) {
  $files += @('nlroom.exe', 'PAYLOAD.sha256', 'Uninstall.exe')
} else {
  $files += @('install.ps1', 'uninstall.ps1', 'setup.ps1', 'NodeLaneRoom.cmd')
  if ($hasGUI) { $files += @('nlroom.exe', 'PAYLOAD.sha256', 'MicrosoftEdgeWebview2Setup.exe') }
}
foreach ($file in $files) {
  if (-not (Test-Path -LiteralPath (Join-Path $source $file) -PathType Leaf)) { throw "Incomplete package: $file" }
}
if (-not (Test-Path -LiteralPath (Join-Path $source 'licenses') -PathType Container)) { throw 'Incomplete package: licenses' }
if ($hasGUI) {
  # Hashes detect incomplete or mixed payloads; test packages are not publisher-signed.
  $hashes = @{}
  foreach ($line in (Get-Content -LiteralPath (Join-Path $source 'PAYLOAD.sha256'))) {
    if ($line -notmatch '^([0-9a-f]{64})  ([a-zA-Z0-9_./@+ -]+)$') { throw 'Invalid payload manifest' }
    $hash = $Matches[1]; $name = $Matches[2]
    if ($name.StartsWith('/') -or $name.Split('/') -contains '..' -or $hashes.ContainsKey($name)) { throw 'Invalid payload path' }
    $hashes[$name] = $hash
  }
  # NSIS creates Uninstall.exe at runtime from its embedded uninstaller code.
  foreach ($name in @($files | Where-Object { $_ -notin @('PAYLOAD.sha256', 'Uninstall.exe') }) + @(Get-ChildItem -LiteralPath (Join-Path $source 'licenses') -Recurse -File | ForEach-Object { $_.FullName.Substring($source.Length + 1).Replace('\', '/') })) {
    if (-not $hashes.ContainsKey($name) -or (Get-FileHash -LiteralPath (Join-Path $source $name) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $hashes[$name]) { throw "Payload verification failed: $name" }
  }
}
$existing = Get-Service -Name NodeLaneRoom -ErrorAction SilentlyContinue
$ownerFile = Join-Path $env:ProgramData 'NodeLaneRoom/owner.sid'
if (Test-Path -LiteralPath $ownerFile) {
  Assert-Tree (Split-Path $ownerFile -Parent) -Trusted
  if ((Get-Content -LiteralPath $ownerFile -Raw).Trim() -ne $OwnerSid) { throw 'This installation belongs to a different player. Run setup from the original player account.' }
}
if ($existing) {
  if (-not (Test-Path -LiteralPath $ownerFile)) { throw 'Installed service has no owner binding' }
  $config = Get-CimInstance Win32_Service -Filter "Name='NodeLaneRoom'"
  $expectedPath = '"' + (Join-Path $target 'nlroom-service.exe') + '"'
  if (-not $config.PathName.StartsWith($expectedPath, [StringComparison]::OrdinalIgnoreCase) -or $config.StartName -ne 'LocalSystem') { throw 'Unexpected service configuration; installation was not changed' }
}
$oldVersion = $null
if (Test-Path -LiteralPath (Join-Path $target 'BUILD.txt')) {
  $oldBuild = Get-Content -LiteralPath (Join-Path $target 'BUILD.txt') -Raw
  if ($oldBuild -notmatch '(?m)^Version: (\d+\.\d+\.\d+)\s*$') { throw 'Installed version cannot be verified' }
  $oldVersion = $Matches[1]
  if (-not $Rollback -and [version]$version -lt [version]$oldVersion) { throw 'Downgrade refused; use the explicit rollback command' }
}
if ($existing -and -not $oldVersion) { throw 'Installed service has no verifiable version; refusing replacement' }
if ($managedGUI) {
  $tapDirectory = Join-Path $PSScriptRoot 'tap'
  . (Join-Path $PSScriptRoot 'tap.ps1')
  Assert-TapPackage $tapDirectory
}
if ($CheckOnly) { Write-Output "Package $version verified. No installation changes made."; return }

# The lock is in administrator-controlled Program Files, shared with uninstall.
$lock = [IO.File]::Open((Join-Path $base 'NodeLaneRoom.install.lock'), 'OpenOrCreate', 'ReadWrite', 'None')
$movedOld = $false; $movedNew = $false; $stopped = $false
$createdTap = $null
$wasRunning = $existing -and $existing.Status -eq 'Running'
try {
  if (-not (Test-Path -LiteralPath $target) -and (Test-Path -LiteralPath $previous)) { throw 'Interrupted installation: restore NodeLaneRoom.previous to NodeLaneRoom before retrying' }
  Remove-Bundle $pending
  New-Item -ItemType Directory -Path $pending | Out-Null
  Protect-Bundle $pending
  foreach ($file in $files) {
    $destination = Join-Path $pending $file
    New-Item -ItemType Directory -Force -Path (Split-Path $destination -Parent) | Out-Null
    Copy-Item -LiteralPath (Join-Path $source $file) -Destination $destination
  }
  Copy-Item -LiteralPath (Join-Path $source 'licenses') -Destination $pending -Recurse
  # Copy-Item does not preserve source ACLs. Explicitly own the copied files as
  # Administrators even on hosts whose creator-owner policy names the UAC user.
  foreach ($item in @(Get-ChildItem -LiteralPath $pending -Recurse -Force)) {
    $itemAcl = Get-Acl -LiteralPath $item.FullName
    $itemAcl.SetOwner([System.Security.Principal.SecurityIdentifier]::new('S-1-5-32-544'))
    Set-Acl -LiteralPath $item.FullName -AclObject $itemAcl
  }
  Assert-Tree $pending -Trusted
  if ($hasGUI) {
    foreach ($name in $hashes.Keys) {
      if ((Get-FileHash -LiteralPath (Join-Path $pending $name) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $hashes[$name]) { throw 'Staged payload changed during installation' }
    }
    $runtimeKey = 'SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
    $runtime = @(
      Get-ItemProperty -LiteralPath ('Registry::HKEY_USERS\' + $OwnerSid + '\' + $runtimeKey) -Name pv -ErrorAction SilentlyContinue
      Get-ItemProperty -LiteralPath ('HKLM:\SOFTWARE\WOW6432Node\' + $runtimeKey.Substring(9)) -Name pv -ErrorAction SilentlyContinue
    ) | Where-Object { $_.pv -and $_.pv -ne '0.0.0.0' }
    if (-not $runtime) {
      $bootstrap = if ($managedGUI) { Join-Path $PSScriptRoot 'MicrosoftEdgeWebview2Setup.exe' } else { Join-Path $pending 'MicrosoftEdgeWebview2Setup.exe' }
      $signed = Get-AuthenticodeSignature -LiteralPath $bootstrap
      if ($signed.Status -ne 'Valid' -or $signed.SignerCertificate.Subject -notmatch 'O=Microsoft Corporation(?:,|$)') { throw 'WebView2 bootstrapper signature verification failed' }
      $setup = Start-Process -FilePath $bootstrap -ArgumentList @('/silent', '/install') -WindowStyle Hidden -PassThru
      if (-not $setup.WaitForExit(300000)) { throw 'WebView2 installation is still running; wait for it to finish before retrying' }
      if ($setup.ExitCode -ne 0) { throw 'Microsoft WebView2 installation failed; check connectivity and retry' }
      $installedRuntime = Get-ItemProperty -LiteralPath ('HKLM:\SOFTWARE\WOW6432Node\' + $runtimeKey.Substring(9)) -Name pv -ErrorAction SilentlyContinue
      if (-not $installedRuntime -or $installedRuntime.pv -eq '0.0.0.0') { throw 'WebView2 is not ready; restart Windows and retry the installer' }
    }
  }
  foreach ($process in @(Get-Process -Name nlroom -ErrorAction SilentlyContinue)) {
    if ($process.Path -eq (Join-Path $target 'nlroom.exe')) { Stop-Process -Id $process.Id -Force; $process.WaitForExit() }
  }
  Stop-Network
  $stopped = $true
  if ($managedGUI) {
    Write-InstallMessage 'Preparing the signed TAP driver and dedicated nodelane0-lan adapter...'
    $createdTap = Install-NodeLaneTap $tapDirectory
  }
  Remove-Bundle $previous
  if (Test-Path -LiteralPath $target) { Move-Bundle $target $previous; $movedOld = $true }
  Move-Bundle $pending $target
  $movedNew = $true
  Write-InstallMessage 'Starting the networking service...'
  if ($existing) { Start-Service -Name NodeLaneRoom } else {
    & (Join-Path $target 'nlroom-service.exe') service install --owner-sid $OwnerSid
    if ($LASTEXITCODE -ne 0) { throw 'Service registration failed' }
  }
  Wait-Ready $version
  Register-Application $version
  if ($createdTap) {
    Assert-Tree (Split-Path $ownerFile -Parent) -Trusted
    Set-Content -LiteralPath (Join-Path (Split-Path $ownerFile -Parent) 'tap.guid') -Value $createdTap -Encoding ascii
  }
  Write-InstallMessage "NodeLane Room $version installed. Open the application as the installation user."
} catch {
  $failure = $_
  if ($movedNew) { Stop-Network }
  $tapCleanupError = $null
  if ($createdTap) {
    try { Remove-NodeLaneTap $tapDirectory $createdTap } catch { $tapCleanupError = $_ }
  }
  if ($movedNew) {
    if (-not $existing -and (Get-Service -Name NodeLaneRoom -ErrorAction SilentlyContinue)) {
      & (Join-Path $target 'nlroom-service.exe') service uninstall
      if ($LASTEXITCODE -ne 0) { throw 'Failed to remove new service; installation files retained for recovery' }
    }
    Remove-Bundle $target
    if (-not $movedOld -and $hasGUI) { Unregister-Application }
  }
  if ($movedOld) {
    Move-Bundle $previous $target
  }
  if ($existing -and $stopped -and $wasRunning) {
    Start-Service -Name NodeLaneRoom
    Wait-Ready $oldVersion
    Write-Output 'Previous networking service restored.'
  }
  if ($movedOld) { Register-Application $oldVersion }
  if ($tapCleanupError) { Write-InstallMessage ('The previous application was restored, but TAP cleanup needs attention: ' + $tapCleanupError.Exception.Message) }
  throw $failure
} finally {
  $lock.Dispose()
}
