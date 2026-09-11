# Exercise the real file-swap/rollback block with simulated service operations.
# All paths stay in a unique workspace temporary directory; no service is installed.
$ErrorActionPreference = 'Stop'
$root = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$text = Get-Content -LiteralPath (Join-Path $root 'scripts/install.ps1') -Raw
$tokens = $null; $parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseInput($text, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors) { throw $parseErrors[0] }
$functions = $ast.FindAll({ param($node) $node -is [System.Management.Automation.Language.FunctionDefinitionAst] }, $false)
$transaction = $text.Substring($text.IndexOf('# The lock is'))
$testRoot = Join-Path $root ('.local/install-test-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot | Out-Null
try {
  foreach ($scenario in @('upgrade', 'start-failure', 'readiness-failure', 'stop-failure', 'rollback', 'registration-failure', 'tap-failure', 'tap-created', 'tap-created-start-failure', 'tap-cleanup-failure')) {
    & {
      foreach ($definition in $functions) { . ([scriptblock]::Create($definition.Extent.Text)) }
      # Windows ACL/DPAPI and real SCM/driver acceptance are separate checks.
      function Assert-Tree { param($Path, [switch]$Trusted) }
      function Protect-Bundle { param($Path) }
      function Get-ChildItem { param($LiteralPath, [switch]$Recurse, [switch]$Force) @() }
      function Get-Process { param($Name, $ErrorAction) @() }
      function Stop-Network {
        if ($scenario -eq 'stop-failure') { throw 'simulated stop failure' }
      }
      function Start-Service {
        param($Name)
        if ($scenario -in @('start-failure', 'tap-created-start-failure', 'tap-cleanup-failure') -and (Get-Content (Join-Path $target 'BUILD.txt')) -eq 'new') { throw 'simulated start failure' }
      }
      function Install-NodeLaneTap {
        param($Directory)
        if ($scenario -eq 'tap-failure') { throw 'simulated TAP failure' }
        if ($scenario -in @('tap-created', 'tap-created-start-failure', 'tap-cleanup-failure')) { return '{a986c01f-4256-4921-bce7-d3d3fb61898b}' }
        return $null
      }
      function Remove-NodeLaneTap {
        param($Directory, $Guid)
        if ($Guid -ne '{a986c01f-4256-4921-bce7-d3d3fb61898b}') { throw 'Wrong adapter cleanup' }
        $script:tapRemoved = $true
        if ($scenario -eq 'tap-cleanup-failure') { throw 'simulated TAP cleanup failure' }
      }
      function Wait-Ready {
        param($Version)
        if ($scenario -eq 'readiness-failure' -and $Version -eq '0.2.1') { throw 'simulated readiness failure' }
      }
      function Register-Application {
        param($Version)
        Set-Content -LiteralPath (Join-Path $base 'registered-version') -Value $Version
        if ($scenario -eq 'registration-failure' -and $Version -eq '0.2.1') { throw 'simulated registration failure' }
      }
      $base = Join-Path $testRoot $scenario
      New-Item -ItemType Directory -Path $base | Out-Null
      $target = Join-Path $base 'NodeLaneRoom'
      $previous = Join-Path $base 'NodeLaneRoom.previous'
      $pending = Join-Path $base 'NodeLaneRoom.pending'
      $source = Join-Path $base 'payload'
      foreach ($folder in @($target, $source)) {
        New-Item -ItemType Directory -Path (Join-Path $folder 'licenses') -Force | Out-Null
      }
      Set-Content -LiteralPath (Join-Path $target 'BUILD.txt') -Value 'old'
      Set-Content -LiteralPath (Join-Path $source 'BUILD.txt') -Value 'new'
      $files = @('BUILD.txt')
      # Exercise the complete GUI payload without touching WebView2 or SCM.
      $hasGUI = $true; $managedGUI = $true
      $hashes = @{'BUILD.txt' = (Get-FileHash -LiteralPath (Join-Path $source 'BUILD.txt')).Hash.ToLowerInvariant()}
      function Get-ItemProperty { param($LiteralPath, $Name, $ErrorAction) [pscustomobject]@{ pv = '1.0.0.0' } }
      $existing = [pscustomobject]@{ Status = 'Running' }
      $ownerFile = Join-Path $base 'state/owner.sid'
      New-Item -ItemType Directory -Path (Split-Path $ownerFile -Parent) | Out-Null
      $script:tapRemoved = $false
      $tapDirectory = Join-Path $base 'tap'
      $oldVersion = '0.2.0'; $version = '0.2.1'
      if ($scenario -eq 'rollback') {
        Move-Item -LiteralPath $source -Destination $previous
        $source = $previous
      }
      $failed = $false
      try { . ([scriptblock]::Create($transaction)) } catch { $failed = $true; $failure = $_ }
      $expectedFailure = $scenario -in @('start-failure', 'readiness-failure', 'stop-failure', 'registration-failure', 'tap-failure', 'tap-created-start-failure', 'tap-cleanup-failure')
      if ($failed -ne $expectedFailure) { throw "Unexpected transaction result: $scenario ($failure)" }
      $want = if ($expectedFailure) { 'old' } else { 'new' }
      if ((Get-Content -LiteralPath (Join-Path $target 'BUILD.txt')) -ne $want) { throw "Wrong active program after $scenario" }
      if (-not $expectedFailure -and (Get-Content -LiteralPath (Join-Path $previous 'BUILD.txt')) -ne 'old') { throw 'Missing previous version' }
      if ($scenario -eq 'registration-failure' -and (Get-Content -LiteralPath (Join-Path $base 'registered-version')) -ne $oldVersion) { throw 'Failed upgrade left the wrong registered version' }
      if ($scenario -eq 'tap-created' -and (Get-Content -LiteralPath (Join-Path $base 'state/tap.guid')) -ne '{a986c01f-4256-4921-bce7-d3d3fb61898b}') { throw 'Created TAP ownership was not recorded' }
      if ($script:tapRemoved -ne ($scenario -in @('tap-created-start-failure', 'tap-cleanup-failure'))) { throw 'Wrong TAP cleanup on installation failure' }
      try { Assert-Bundle (Join-Path $base '../outside'); throw 'Accepted unsafe path' } catch {
        if ($_.Exception.Message -ne 'Unsafe installation path') { throw }
      }
      Write-Output "PASS installer transaction: $scenario"
    }
  }
} finally {
  $resolved = [IO.Path]::GetFullPath($testRoot)
  $allowed = [IO.Path]::GetFullPath((Join-Path $root '.local'))
  if ((Split-Path $resolved -Parent) -ne $allowed -or (Split-Path $resolved -Leaf) -notlike 'install-test-*') { throw 'Unsafe test cleanup path' }
  Remove-Item -LiteralPath $resolved -Recurse -Force
}
