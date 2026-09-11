# Use Windows PowerShell 5.1 and a harmless child process; never request UAC.
$ErrorActionPreference = 'Stop'
$root = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$ast = [Management.Automation.Language.Parser]::ParseFile((Join-Path $root 'scripts/setup.ps1'), [ref]$null, [ref]$null)
$definition = $ast.Find({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Invoke-ElevatedInstaller' }, $false)
. ([scriptblock]::Create($definition.Extent.Text))
$testRoot = Join-Path $root ('.local/setup-test-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot | Out-Null
function Start-Process {
  param($FilePath, $Verb, $WindowStyle, [switch]$PassThru, $ArgumentList)
  if ($Verb -ne 'RunAs' -or $WindowStyle -ne 'Hidden') { throw 'Missing installer elevation arguments' }
  # Deliberately omit RunAs for this fake installer.
  Microsoft.PowerShell.Management\Start-Process -FilePath $FilePath -WindowStyle Hidden -PassThru -ArgumentList $ArgumentList
}
try {
  foreach ($scenario in @('success', 'failure', 'early-exit')) {
    $installer = Join-Path $testRoot ($scenario + '.ps1')
    $code = @'
param($OwnerSid, $SourceDir, $LogPipe, [switch]$Rollback, [switch]$Quiet)
$pipe = [IO.Pipes.NamedPipeClientStream]::new('.', $LogPipe, [IO.Pipes.PipeDirection]::Out)
$pipe.Connect(10000)
$writer = [IO.StreamWriter]::new($pipe, [Text.UTF8Encoding]::new($false))
$writer.AutoFlush = $true
$writer.WriteLine('Detailed installer status')
$writer.Dispose()
'@
    if ($scenario -eq 'early-exit') { $code = 'exit 3' }
    if ($scenario -eq 'failure') { $code += "`nexit 1" }
    Set-Content -LiteralPath $installer -Value $code -Encoding UTF8
    $messages = [Collections.Generic.List[string]]::new()
    $failure = $null
    try {
      Invoke-ElevatedInstaller $installer ([Security.Principal.WindowsIdentity]::GetCurrent().User.Value) $testRoot -Quiet | ForEach-Object { $messages.Add($_) }
    } catch { $failure = $_.Exception.Message }
    if ($scenario -ne 'early-exit' -and -not $messages.Contains('Detailed installer status')) { throw 'Elevated error detail was lost' }
    if ($scenario -eq 'success' -and $failure) { throw $failure }
    if ($scenario -eq 'failure' -and $failure -notlike '*code 1*') { throw "Wrong installer failure: $failure" }
    if ($scenario -eq 'early-exit' -and $failure -notlike '*before reporting*code 3*') { throw "Wrong early exit: $failure" }
    Write-Output "PASS setup status pipe: $scenario"
  }
} finally {
  $resolved = [IO.Path]::GetFullPath($testRoot)
  if ((Split-Path $resolved -Parent) -ne [IO.Path]::GetFullPath((Join-Path $root '.local')) -or (Split-Path $resolved -Leaf) -notlike 'setup-test-*') { throw 'Unsafe test cleanup' }
  Remove-Item -LiteralPath $resolved -Recurse -Force
}
