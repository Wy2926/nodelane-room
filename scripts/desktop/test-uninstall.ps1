# Execute real uninstall orchestration with simulated SCM and temporary files only.
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.ServiceProcess
$root = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$tokens = $null; $errors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile((Join-Path $root 'scripts/uninstall.ps1'), [ref]$tokens, [ref]$errors)
if ($errors) { throw $errors[0] }
$functions = $ast.FindAll({ param($node) $node -is [System.Management.Automation.Language.FunctionDefinitionAst] }, $false)
$testRoot = Join-Path $root ('.local/uninstall-test-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot | Out-Null
try {
  foreach ($scenario in @('preserve', 'purge', 'autostart', 'stop-failure', 'live-process', 'remove-failure', 'unsafe-backup', 'foreign-service')) {
    & {
      foreach ($definition in $functions) { . ([scriptblock]::Create($definition.Extent.Text)) }
      $base = Join-Path $testRoot $scenario
      $programFiles = Join-Path $base 'programs'
      $target = Join-Path $programFiles 'NodeLaneRoom'
      $state = Join-Path $env:ProgramData 'NodeLaneRoom'
      $stateRemoved = $false; $entryRemoved = $false; $stopCalled = $false
      foreach ($name in @('NodeLaneRoom', 'NodeLaneRoom.previous', 'NodeLaneRoom.pending')) {
        New-Item -ItemType Directory -Path (Join-Path $programFiles $name) -Force | Out-Null
      }
      Set-Content -LiteralPath (Join-Path $target 'Uninstall.exe') -Value 'fixture'
      function Assert-RemovalTree {
        param($Path, $Parent)
        if ((Split-Path ([IO.Path]::GetFullPath($Path)) -Parent) -ne $Parent) { throw 'Unsafe removal path' }
        if ($scenario -eq 'unsafe-backup' -and $Path.EndsWith('.previous')) { throw 'simulated unsafe backup' }
      }
      function Get-Service {
        param($Name, $ErrorAction)
        $service = [pscustomobject]@{ Status = 'Running' }
        $service | Add-Member ScriptMethod WaitForStatus { param($Status, $Timeout) }
        return $service
      }
      function Get-CimInstance {
        param($ClassName, $Filter)
        $path = if ($scenario -eq 'foreign-service') { '"C:\Other\service.exe"' } else { '"' + (Join-Path $target 'nlroom-service.exe') + '"' }
        [pscustomobject]@{ PathName = $path; StartName = 'LocalSystem' }
      }
      function Stop-Service {
        param($Name, $ErrorAction)
        $script:stopCalled = $true
        if ($scenario -eq 'stop-failure') { throw 'simulated stop failure' }
      }
      function Get-Process {
        param($Name, $ErrorAction)
        if ($Name -eq 'nlroom-service' -and $scenario -eq 'live-process') {
          $process = [pscustomobject]@{ Path = (Join-Path $target 'nlroom-service.exe') }
          $process | Add-Member ScriptMethod WaitForExit { param($Timeout) return $false }
          $process
        }
      }
      # Override only this exact executable command; never launch a real service.
      $serviceCommand = Join-Path $target 'nlroom-service.exe'
      Set-Item -Path ('Function:' + $serviceCommand) -Value {
        if ($scenario -eq 'remove-failure') { $global:LASTEXITCODE = 1 } else { $global:LASTEXITCODE = 0 }
      }
      function Test-Path {
        param($LiteralPath)
        if ($LiteralPath -eq $state -or $LiteralPath -like 'HKLM:*') { return $true }
        if ($scenario -eq 'autostart' -and ($LiteralPath -eq (Join-Path $state 'owner.sid') -or $LiteralPath -like 'Registry::HKEY_USERS\S-1-5-21-1-2-3-1001\*')) { return $true }
        if (-not $LiteralPath.StartsWith($base)) { return $false }
        Microsoft.PowerShell.Management\Test-Path -LiteralPath $LiteralPath
      }
      function Get-Content {
        param($LiteralPath, [switch]$Raw)
        if ($LiteralPath -ne (Join-Path $state 'owner.sid')) { throw 'Unexpected test read' }
        'S-1-5-21-1-2-3-1001'
      }
      function Get-Item {
        param($LiteralPath)
        if ($LiteralPath -notlike 'Registry::HKEY_USERS\S-1-5-21-1-2-3-1001\*') { throw 'Wrong startup user' }
        $key = [pscustomobject]@{}
        $key | Add-Member ScriptMethod GetValueNames { @('NodeLane Room', 'OtherApp') }
        $key | Add-Member ScriptMethod GetValue { param($Name) if ($Name -eq 'NodeLane Room') { '"' + (Join-Path $target 'nlroom.exe') + '"' } else { '"C:\Other\other.exe"' } }
        $key
      }
      function Remove-ItemProperty {
        param($LiteralPath, $Name, $ErrorAction)
        if ($Name -ne 'NodeLane Room' -or $LiteralPath -notlike 'Registry::HKEY_USERS\S-1-5-21-1-2-3-1001\*') { throw 'Removed unrelated startup data' }
        $script:startupRemoved++
      }
      function Remove-Item {
        param($LiteralPath, [switch]$Recurse, [switch]$Force)
        if ($LiteralPath -eq $state) { $script:stateRemoved = $true; return }
        if ($LiteralPath -like 'HKLM:*') { $script:entryRemoved = $true; return }
        if (-not ([IO.Path]::GetFullPath($LiteralPath)).StartsWith($base + '\')) { throw 'Unsafe test deletion' }
        Microsoft.PowerShell.Management\Remove-Item -LiteralPath $LiteralPath -Recurse:$Recurse -Force:$Force
      }
      $PurgeState = $scenario -eq 'purge'
      $script:stateRemoved = $false; $script:entryRemoved = $false; $script:stopCalled = $false
      $script:startupRemoved = 0
      $failed = $false
      $failure = $null
      try { Remove-Application } catch { $failed = $true; $failure = $_ }
      $expectedFailure = $scenario -notin @('preserve', 'purge', 'autostart')
      if ($failed -ne $expectedFailure) { throw "Unexpected uninstall result: $scenario ($failure)" }
      if ((Test-Path -LiteralPath (Join-Path $target 'Uninstall.exe')) -ne $expectedFailure) { throw "Wrong retained files: $scenario" }
      if ($script:entryRemoved -eq $expectedFailure) { throw "Wrong retained application entry: $scenario" }
      if ($script:stateRemoved -ne $PurgeState) { throw "Wrong identity preservation: $scenario" }
      if ($scenario -eq 'autostart' -and $script:startupRemoved -ne 2) { throw 'Bound player startup entry was not removed' }
      if ($scenario -in @('unsafe-backup', 'foreign-service') -and $script:stopCalled) { throw 'Stopped a service before validation' }
      Microsoft.PowerShell.Management\Remove-Item -LiteralPath ('Function:' + $serviceCommand) -ErrorAction SilentlyContinue
      Write-Output "PASS uninstall: $scenario"
    }
  }
} finally {
  $resolved = [IO.Path]::GetFullPath($testRoot)
  if ((Split-Path $resolved -Parent) -ne [IO.Path]::GetFullPath((Join-Path $root '.local')) -or (Split-Path $resolved -Leaf) -notlike 'uninstall-test-*') { throw 'Unsafe test cleanup' }
  Microsoft.PowerShell.Management\Remove-Item -LiteralPath $resolved -Recurse -Force
}
