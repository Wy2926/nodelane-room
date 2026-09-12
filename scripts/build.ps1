param(
  [string]$Go = 'go',
  [string]$UpdateRoot = '',
  [ValidateSet('control','node','client')]
  [string[]]$Components = @('control','node','client'),
  [ValidateSet('windows/amd64','windows/arm64','linux/amd64','linux/arm64')]
  [string[]]$Targets = @('windows/amd64','windows/arm64','linux/amd64','linux/arm64')
)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
Set-Location -LiteralPath $root
$versionSource = Get-Content -LiteralPath (Join-Path $root 'internal/model/version.go') -Raw
$versions = @{}
foreach ($role in @('control','node','client')) {
  if ($versionSource -notmatch ('const ' + $role + 'Version = "(\d+\.\d+\.\d+)"')) { throw "Missing $role version" }
  $versions[$role] = $Matches[1]
}
# The control image serves the pinned node installers for both Linux architectures.
if ($Components -contains 'control') { $Components += 'node' }
function Invoke-Go { & $Go @args; if ($LASTEXITCODE -ne 0) { throw "go failed with exit code $LASTEXITCODE" } }
$buildFlags = '-s -w'
if ($UpdateRoot) {
  $trust = [Convert]::ToBase64String([IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $UpdateRoot)))
  $buildFlags += ' -X github.com/nodelane/nodelane-room/internal/update.TrustedRootBase64=' + $trust
}
Invoke-Go mod verify
if ($Components -contains 'control') {
  & npm.cmd --prefix internal/control/adminweb ci
  if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
  & npm.cmd --prefix internal/control/adminweb run build
  if ($LASTEXITCODE -ne 0) { throw 'admin build failed' }
}
$goRoot=(Invoke-Go env GOROOT).Trim()
$nebulaModule = (Invoke-Go list -m -f '{{.Path}} {{.Version}}{{with .Replace}} => {{.Path}} {{.Version}}{{end}}' github.com/slackhq/nebula).Trim()
$previous = @{ GOOS=$env:GOOS; GOARCH=$env:GOARCH; CGO_ENABLED=$env:CGO_ENABLED }
try {
  $env:CGO_ENABLED = '0'
  foreach ($role in @('node','client','control')) {
    if ($Components -notcontains $role) { continue }
    $Version = $versions[$role]
    $release = Join-Path $root "dist/$role/$Version"
    New-Item -ItemType Directory -Force -Path $release | Out-Null
    $roleTargets = @($Targets | Where-Object { $role -eq 'client' -or $_.StartsWith('linux/') } | Select-Object -Unique)
    if ($role -eq 'node' -and $Components -contains 'control') { $roleTargets = @('linux/amd64','linux/arm64') }
    foreach ($target in $roleTargets) {
      $parts = $target.Split('/')
      $env:GOOS = $parts[0]; $env:GOARCH = $parts[1]
      $name = "nodelane-room-$role-$Version-$($parts[0])-$($parts[1])"
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
      $commands = switch ($role) {
        'control' { @('nodelane-server') }
        'node' { @('nlroom-node') }
        'client' { @('nlroom-cli','nlroom-service','nlroom-update') }
      }
      foreach ($command in $commands) {
        $filename = $command
        if ($env:GOOS -eq 'windows') { $filename += '.exe' }
        Invoke-Go build -trimpath -buildvcs=false -ldflags $buildFlags -o (Join-Path $bundle $filename) "./cmd/$command"
      }
      $packagePaths = @($commands | ForEach-Object { "./cmd/$_" })
      $moduleList = Invoke-Go list -deps -f '{{if .Module}}{{if .Module.Replace}}{{.Module.Replace.Path}}|{{.Module.Replace.Version}}{{else}}{{.Module.Path}}|{{.Module.Version}}{{end}}|{{.Module.Dir}}{{end}}' @packagePaths | Where-Object { $_ -ne '' } | Sort-Object -Unique
      # User/developer documentation stays in the source tree. Keep legal notices.
      Copy-Item -LiteralPath (Join-Path $root 'THIRD_PARTY_NOTICES.md') -Destination (Join-Path $bundle 'THIRD_PARTY_NOTICES.txt') -Force
      if ($role -eq 'control') {
        $deployOut=Join-Path $bundle 'deploy'
        New-Item -ItemType Directory -Force -Path $deployOut | Out-Null
        foreach ($template in @('compose.yaml','compose.host.yaml','compose.network.yaml','compose.node.yaml','compose.build.yaml','compose.node.build.yaml','Caddyfile','.env.example','.env.host.example','.env.node.example','nlroom-node.service','QUICKSTART.txt')) {
          Copy-Item -LiteralPath (Join-Path $root "deploy/$template") -Destination $deployOut -Force
        }
        # Include digests only when both independently versioned images match.
        $imageList = Join-Path $root 'deploy/IMAGES.txt'
        $published = Get-Content -LiteralPath $imageList -Raw
        if ($published.Contains("Control: $($versions.control)") -and $published.Contains("Node: $($versions.node)")) {
          Copy-Item -LiteralPath $imageList -Destination $deployOut -Force
        }
        foreach ($template in @('.env.example','.env.host.example','.env.node.example')) {
          $envTemplate = Join-Path $deployOut $template
          $templateContent = (Get-Content -LiteralPath $envTemplate -Encoding utf8) -replace '^(# )?NODELANE_CONTROL_VERSION=.*$', "NODELANE_CONTROL_VERSION=$($versions.control)" -replace '^(# )?NODELANE_NODE_VERSION=.*$', "NODELANE_NODE_VERSION=$($versions.node)"
          [IO.File]::WriteAllLines($envTemplate, $templateContent, [Text.UTF8Encoding]::new($false))
        }
        Copy-Item -LiteralPath (Join-Path $root 'deploy/Dockerfile.release') -Destination (Join-Path $bundle 'Dockerfile') -Force
        Copy-Item -LiteralPath (Join-Path $root '.dockerignore') -Destination $bundle -Force
        Copy-Item -LiteralPath (Join-Path $root "dist/node/$($versions.node)/releases") -Destination $bundle -Recurse -Force
      }
      $licenses = Join-Path $bundle 'licenses'
      Invoke-Go version | Set-Content -LiteralPath (Join-Path $bundle 'BUILD.txt') -Encoding utf8
      "Component: $role`nVersion: $Version`nTarget: $target`nNebula: $nebulaModule`nCGO_ENABLED: 0" | Add-Content -LiteralPath (Join-Path $bundle 'BUILD.txt') -Encoding utf8
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
      if ($role -eq 'node') {
        Copy-Item -LiteralPath (Join-Path $root 'deploy/Dockerfile.release') -Destination (Join-Path $bundle 'Dockerfile') -Force
        Copy-Item -LiteralPath (Join-Path $root '.dockerignore') -Destination $bundle -Force
        Copy-Item -LiteralPath (Join-Path $root 'deploy/nlroom-node.service') -Destination $bundle -Force
      }
      if ($env:GOOS -eq 'windows') {
        foreach ($script in @('Install.cmd','NodeLaneRoom.cmd')) {
          Copy-Item -LiteralPath (Join-Path $root "scripts/$script") -Destination $bundle -Force
        }
        $payloadHashes = Get-ChildItem -LiteralPath $bundle -Recurse -File | Sort-Object FullName | ForEach-Object {
          $relative = $_.FullName.Substring($bundle.Length + 1).Replace('\', '/')
          (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + $relative
        }
        $payloadHashes | Set-Content -LiteralPath (Join-Path $bundle 'PAYLOAD.sha256') -Encoding ascii
        Compress-Archive -Path "$bundle/*" -DestinationPath (Join-Path $release "$name.zip") -Force
      } else {
        $targetOS=$env:GOOS; $targetArch=$env:GOARCH
        $env:GOOS=$previous.GOOS; $env:GOARCH=$previous.GOARCH
        Invoke-Go run ./scripts/package -source $bundle -output (Join-Path $release "$name.tar.gz")
        $env:GOOS=$targetOS; $env:GOARCH=$targetArch
      }
    }
    $env:GOOS=$previous.GOOS; $env:GOARCH=$previous.GOARCH
    if ($role -eq 'node' -and $roleTargets -contains 'linux/amd64' -and $roleTargets -contains 'linux/arm64') {
      Invoke-Go run ./scripts/release -release $release -version $Version
    }
    Get-ChildItem -LiteralPath $release -File | Where-Object { $_.Name -match '\.(zip|tar.gz)$' } | ForEach-Object {
      $hash = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
      "$hash  $($_.Name)"
    } | Set-Content -LiteralPath (Join-Path $release 'SHA256SUMS') -Encoding ascii
    Write-Output "Artifacts: $release"
  }
} finally {
  $env:GOOS=$previous.GOOS; $env:GOARCH=$previous.GOARCH; $env:CGO_ENABLED=$previous.CGO_ENABLED
}
