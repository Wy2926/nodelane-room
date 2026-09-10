param(
  [string]$Go = 'go',
  [string]$Version = '0.2.0',
  [ValidateSet('windows/amd64','windows/arm64','linux/amd64','linux/arm64')]
  [string[]]$Targets = @('windows/amd64','windows/arm64','linux/amd64','linux/arm64')
)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
Set-Location -LiteralPath $root
if ($Version -notmatch '^[0-9A-Za-z._-]+$') { throw 'Invalid version' }
function Invoke-Go { & $Go @args; if ($LASTEXITCODE -ne 0) { throw "go failed with exit code $LASTEXITCODE" } }
Invoke-Go mod verify
& npm.cmd --prefix internal/control/adminweb ci
if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
& npm.cmd --prefix internal/control/adminweb run build
if ($LASTEXITCODE -ne 0) { throw 'admin build failed' }
$goRoot=(Invoke-Go env GOROOT).Trim()
$release = Join-Path $root "dist/$Version"
New-Item -ItemType Directory -Force -Path $release | Out-Null
$moduleDir = (Invoke-Go list -m -f '{{.Dir}}' github.com/slackhq/nebula).Trim()
$nebulaModule = (Invoke-Go list -m -f '{{.Path}} {{.Version}}{{with .Replace}} => {{.Path}} {{.Version}}{{end}}' github.com/slackhq/nebula).Trim()
$previous = @{ GOOS=$env:GOOS; GOARCH=$env:GOARCH; CGO_ENABLED=$env:CGO_ENABLED }
try {
  $env:CGO_ENABLED = '0'
  foreach ($target in ($Targets | Select-Object -Unique)) {
    $parts = $target.Split('/')
    $env:GOOS = $parts[0]; $env:GOARCH = $parts[1]
    $name = "nodelane-room-$Version-$($parts[0])-$($parts[1])"
    $bundle = Join-Path $release $name
    # Rebuild a clean, explicitly bounded bundle to avoid stale files/secrets
    # from a previous run leaking into a release archive.
    $resolvedBundle = [IO.Path]::GetFullPath($bundle)
    $resolvedRelease = [IO.Path]::GetFullPath($release)
    if ((Split-Path $resolvedBundle -Parent) -ne $resolvedRelease -or (Split-Path $resolvedBundle -Leaf) -ne $name) { throw 'Unsafe bundle path' }
    if (Test-Path -LiteralPath $resolvedBundle) {
      if ((Get-Item -LiteralPath $resolvedBundle).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Refusing bundle reparse point' }
      Remove-Item -LiteralPath $resolvedBundle -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $bundle | Out-Null
    $commands = @('nodelane', 'nodelane-server', 'nlroom-node')
    if ($env:GOOS -eq 'windows') { $commands = @('nodelane') }
    foreach ($command in $commands) {
      $filename = $command
      if ($env:GOOS -eq 'windows') { $filename += '.exe' }
      Invoke-Go build -trimpath -buildvcs=false -ldflags '-s -w' -o (Join-Path $bundle $filename) "./cmd/$command"
    }
    $packagePaths = @($commands | ForEach-Object { "./cmd/$_" })
    $moduleList = Invoke-Go list -deps -f '{{if .Module}}{{if .Module.Replace}}{{.Module.Replace.Path}}|{{.Module.Replace.Version}}{{else}}{{.Module.Path}}|{{.Module.Version}}{{end}}|{{.Module.Dir}}{{end}}' @packagePaths | Where-Object { $_ -ne '' } | Sort-Object -Unique
    # User/developer documentation stays in the source tree. Keep legal notices.
    Copy-Item -LiteralPath (Join-Path $root 'THIRD_PARTY_NOTICES.md') -Destination (Join-Path $bundle 'THIRD_PARTY_NOTICES.txt') -Force
    if ($env:GOOS -eq 'linux') {
      $deployOut=Join-Path $bundle 'deploy'
      New-Item -ItemType Directory -Force -Path $deployOut | Out-Null
      foreach ($template in @('compose.yaml','compose.host.yaml','compose.network.yaml','compose.node.yaml','compose.build.yaml','compose.node.build.yaml','Caddyfile','.env.example','.env.host.example','.env.node.example','nlroom-node.service','QUICKSTART.txt')) {
        Copy-Item -LiteralPath (Join-Path $root "deploy/$template") -Destination $deployOut -Force
      }
      # A digest list is meaningful only for the exact version actually published.
      $imageList = Join-Path $root 'deploy/IMAGES.txt'
      if ((Get-Content -LiteralPath $imageList -TotalCount 1) -eq "NodeLane Room $Version") {
        Copy-Item -LiteralPath $imageList -Destination $deployOut -Force
      }
      foreach ($template in @('.env.example','.env.host.example','.env.node.example')) {
        $envTemplate = Join-Path $deployOut $template
        $templateContent = (Get-Content -LiteralPath $envTemplate -Encoding utf8) -replace '^(# )?NODELANE_VERSION=.*$', "NODELANE_VERSION=$Version"
        [IO.File]::WriteAllLines($envTemplate, $templateContent, [Text.UTF8Encoding]::new($false))
      }
      Copy-Item -LiteralPath (Join-Path $root 'deploy/Dockerfile.release') -Destination (Join-Path $bundle 'Dockerfile') -Force
      Copy-Item -LiteralPath (Join-Path $root '.dockerignore') -Destination $bundle -Force
    }
    $licenses = Join-Path $bundle 'licenses'
    Invoke-Go version | Set-Content -LiteralPath (Join-Path $bundle 'BUILD.txt') -Encoding utf8
    "Version: $Version`nTarget: $target`nNebula: $nebulaModule`nCGO_ENABLED: 0" | Add-Content -LiteralPath (Join-Path $bundle 'BUILD.txt') -Encoding utf8
    New-Item -ItemType Directory -Force -Path $licenses | Out-Null
    Copy-Item -LiteralPath (Join-Path $goRoot 'LICENSE') -Destination (Join-Path $licenses 'Go-LICENSE') -Force
    $moduleList | ForEach-Object { $m = $_.Split('|'); "$($m[0]) $($m[1])" } | Set-Content -LiteralPath (Join-Path $licenses 'modules.txt') -Encoding utf8
    foreach ($line in $moduleList) {
      $fields = $line.Split('|')
      if ($fields.Count -lt 3 -or $fields[1] -eq '' -or $fields[2] -eq '') { continue }
      $destination = Join-Path $licenses (($fields[0] -replace '[^a-zA-Z0-9._-]', '_') + '@' + $fields[1])
      New-Item -ItemType Directory -Force -Path $destination | Out-Null
      Get-ChildItem -LiteralPath $fields[2] -File | Where-Object { $_.Name -match '^(LICENSE|COPYING|NOTICE|PATENTS)' } | Copy-Item -Destination $destination -Force
    }
    if ($env:GOOS -eq 'windows') {
      $driverDir = Join-Path $bundle "dist/windows/wintun/bin/$($env:GOARCH)"
      New-Item -ItemType Directory -Force -Path $driverDir | Out-Null
      Copy-Item -LiteralPath (Join-Path $moduleDir "dist/windows/wintun/bin/$($env:GOARCH)/wintun.dll") -Destination $driverDir -Force
      Get-ChildItem -LiteralPath (Join-Path $moduleDir 'dist/windows/wintun') -File | Where-Object { $_.Name -match '^(LICENSE|COPYING|NOTICE|PATENTS)' } | Copy-Item -Destination (Join-Path $bundle 'dist/windows/wintun') -Force
      foreach ($script in @('install.ps1','uninstall.ps1','setup.ps1','Install.cmd','NodeLane.cmd')) {
        Copy-Item -LiteralPath (Join-Path $root "scripts/$script") -Destination $bundle -Force
      }
      Compress-Archive -Path "$bundle/*" -DestinationPath (Join-Path $release "$name.zip") -Force
    } else {
      $targetOS=$env:GOOS; $targetArch=$env:GOARCH
      $env:GOOS=$previous.GOOS; $env:GOARCH=$previous.GOARCH
      Invoke-Go run ./scripts/package -source $bundle -output (Join-Path $release "$name.tar.gz")
      $env:GOOS=$targetOS; $env:GOARCH=$targetArch
    }
  }
  $env:GOOS=$previous.GOOS; $env:GOARCH=$previous.GOARCH
  if (($Targets -contains 'linux/amd64') -and ($Targets -contains 'linux/arm64')) {
    Invoke-Go run ./scripts/release -release $release -version $Version
    foreach ($arch in @('amd64','arm64')) {
      $bundleName="nodelane-room-$Version-linux-$arch"
      Invoke-Go run ./scripts/package -source (Join-Path $release $bundleName) -output (Join-Path $release "$bundleName.tar.gz")
    }
  }
  Get-ChildItem -LiteralPath $release -File | Where-Object { $_.Name -match '\.(zip|tar.gz)$' } | ForEach-Object {
    $hash = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $($_.Name)"
  } | Set-Content -LiteralPath (Join-Path $release 'SHA256SUMS') -Encoding ascii
} finally {
  $env:GOOS=$previous.GOOS; $env:GOARCH=$previous.GOARCH; $env:CGO_ENABLED=$previous.CGO_ENABLED
}
Write-Output "Artifacts: $release"
