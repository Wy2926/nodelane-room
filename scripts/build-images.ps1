# Build verified component archives; -Push publishes only explicit version tags.
param(
  [string]$Registry = 'docker.nodelane.net',
  [ValidateSet('control','node')][string[]]$Components = @('control','node'),
  [switch]$Push
)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
if ($Registry -notmatch '^[a-zA-Z0-9.-]+(:[0-9]+)?(/[a-z0-9._-]+)*$') { throw 'Invalid registry' }
$source = Get-Content -LiteralPath (Join-Path $root 'internal/model/version.go') -Raw
foreach ($role in ($Components | Select-Object -Unique)) {
  if ($source -notmatch ('const ' + $role + 'Version = "(\d+\.\d+\.\d+)"')) { throw "Missing $role version" }
  $version = $Matches[1]
  $release = Join-Path $root "dist/$role/$version"
  $context = [IO.Path]::GetFullPath((Join-Path $root "dist/images/$role/$version"))
  $parent = [IO.Path]::GetFullPath((Join-Path $root "dist/images/$role"))
  if ((Split-Path $context -Parent) -ne $parent -or (Split-Path $context -Leaf) -ne $version) { throw 'Unsafe image context' }
  if (Test-Path -LiteralPath $context) {
    if ((Get-Item -LiteralPath $context).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Refusing context reparse point' }
    Remove-Item -LiteralPath $context -Recurse -Force
  }
  New-Item -ItemType Directory -Path $context -Force | Out-Null
  # All image content, including node downloads, comes from checksummed archives.
  $extractor = @'
import hashlib, pathlib, sys, tarfile
release, output = map(pathlib.Path, sys.argv[1:3])
role, version = sys.argv[3:5]
checksums = dict(line.split()[::-1] for line in (release / 'SHA256SUMS').read_text(encoding='ascii').splitlines())
binary = {'control': 'nodelane-server', 'node': 'nlroom-node'}[role]
for arch in ('amd64', 'arm64'):
    name = f'nodelane-room-{role}-{version}-linux-{arch}'
    archive = release / (name + '.tar.gz')
    if hashlib.sha256(archive.read_bytes()).hexdigest() != checksums[archive.name]:
        raise ValueError('Release checksum mismatch: ' + archive.name)
    seen = set()
    with tarfile.open(archive, 'r:gz') as tf:
        for entry in tf.getmembers():
            relative = pathlib.PurePosixPath(entry.name).relative_to(name)
            if '..' in relative.parts or entry.name in seen or not (entry.isfile() or entry.isdir()):
                raise ValueError('Unsafe archive entry')
            seen.add(entry.name)
            if not entry.isfile():
                continue
            if str(relative) in (binary, 'BUILD.txt', 'THIRD_PARTY_NOTICES.txt') or relative.parts[0] == 'licenses':
                target = output / arch / str(relative)
            elif role == 'control' and relative.parts[0] == 'releases':
                target = output / str(relative)
                if arch == 'arm64' and (not target.is_file() or target.read_bytes() != tf.extractfile(entry).read()):
                    raise ValueError('Node downloads differ between control architectures')
            else:
                continue
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(tf.extractfile(entry).read())
    if not (output / arch / binary).is_file():
        raise ValueError('Missing binary: ' + binary)
    build = (output / arch / 'BUILD.txt').read_text(encoding='utf-8-sig').splitlines()
    if f'Component: {role}' not in build or f'Version: {version}' not in build or f'Target: linux/{arch}' not in build:
        raise ValueError('Release metadata mismatch')
if role == 'control':
    native = output / 'releases'
    for line in (native / 'SHA256SUMS').read_text(encoding='ascii').splitlines():
        digest, filename = line.split()
        if pathlib.PurePosixPath(filename).name != filename or hashlib.sha256((native / filename).read_bytes()).hexdigest() != digest:
            raise ValueError('Node download checksum mismatch')
'@
  $extractor | python - $release $context $role $version
  if ($LASTEXITCODE -ne 0) { throw 'Release extraction failed' }
  Copy-Item -LiteralPath (Join-Path $root 'deploy/Dockerfile.registry') -Destination (Join-Path $context 'Dockerfile') -Force
  @('**', '!Dockerfile', '!amd64/', '!arm64/', '!releases/', '!releases/**', '!*/nodelane-server', '!*/nlroom-node', '!*/BUILD.txt', '!*/THIRD_PARTY_NOTICES.txt', '!*/licenses/', '!*/licenses/**') | Set-Content -LiteralPath (Join-Path $context '.dockerignore') -Encoding ascii
  $tag = "$Registry/nodelane-room-$($role):$version"
  docker buildx build --platform linux/amd64,linux/arm64 --target $role --build-arg "VERSION=$version" --tag $tag --load --metadata-file (Join-Path $context 'metadata.json') $context
  if ($LASTEXITCODE -ne 0) { throw "Image build failed: $tag" }
  if ($Push) {
    docker push $tag
    if ($LASTEXITCODE -ne 0) { throw "Image push failed: $tag" }
  }
}
