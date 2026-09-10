# Build the exact Windows-produced Linux release binaries into multi-platform images.
# -Push explicitly publishes the two versioned tags; no latest tag is created.
param([string]$Version = '0.2.0', [string]$Registry = 'docker.nodelane.net', [switch]$Push)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
if ($Version -notmatch '^[0-9A-Za-z._-]+$') { throw 'Invalid version' }
if ($Registry -notmatch '^[a-zA-Z0-9.-]+(:[0-9]+)?(/[a-z0-9._-]+)*$') { throw 'Invalid registry' }
$release = Join-Path $root "dist/$Version"
$context = Join-Path $root "dist/images/$Version"
New-Item -ItemType Directory -Path $context -Force | Out-Null
# Extract an explicit allowlist from the checksummed archives, not from mutable loose binaries.
$extractor = @'
import hashlib, pathlib, sys, tarfile
release, output, version = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2]), sys.argv[3]
checksums = dict(line.split()[::-1] for line in (release / 'SHA256SUMS').read_text(encoding='ascii').splitlines())
for arch in ('amd64', 'arm64'):
    name = f'nodelane-room-{version}-linux-{arch}'
    archive = release / (name + '.tar.gz')
    if hashlib.sha256(archive.read_bytes()).hexdigest() != checksums[archive.name]:
        raise ValueError('Release checksum mismatch: ' + archive.name)
    with tarfile.open(archive, 'r:gz') as tf:
        for entry in tf.getmembers():
            if not entry.isfile():
                continue
            relative = pathlib.PurePosixPath(entry.name).relative_to(name)
            if '..' in relative.parts:
                raise ValueError('Unsafe archive path')
            if str(relative) in ('nlroom-node', 'nodelane-server', 'BUILD.txt', 'THIRD_PARTY_NOTICES.txt') or relative.parts[0] == 'licenses':
                target = output / arch / str(relative)
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(tf.extractfile(entry).read())
    for binary in ('nlroom-node', 'nodelane-server'):
        if not (output / arch / binary).is_file():
            raise ValueError('Missing binary: ' + binary)
'@
$extractor | python - $release $context $Version
if ($LASTEXITCODE -ne 0) { throw 'Release extraction failed' }
Copy-Item -LiteralPath (Join-Path $root 'deploy/Dockerfile.registry') -Destination (Join-Path $context 'Dockerfile') -Force
Copy-Item -LiteralPath (Join-Path $release 'releases') -Destination $context -Recurse -Force
# Only explicit payload paths are ever sent to the builder.
@('**', '!Dockerfile', '!amd64/', '!arm64/', '!releases/', '!releases/**', '!*/nodelane-server', '!*/nlroom-node', '!*/BUILD.txt', '!*/THIRD_PARTY_NOTICES.txt', '!*/licenses/', '!*/licenses/**') | Set-Content -LiteralPath (Join-Path $context '.dockerignore') -Encoding ascii
foreach ($role in @('control', 'node')) {
  $tag = "$Registry/nodelane-room-${role}:$Version"
  docker buildx build --platform linux/amd64,linux/arm64 --target $role --build-arg "VERSION=$Version" --tag $tag --load --metadata-file (Join-Path $context "$role-metadata.json") $context
  if ($LASTEXITCODE -ne 0) { throw "Image build failed: $tag" }
  if ($Push) {
    docker push $tag
    if ($LASTEXITCODE -ne 0) { throw "Image push failed: $tag" }
  }
}
