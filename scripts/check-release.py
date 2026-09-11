#!/usr/bin/env python3
"""Check release hashes, payloads, architectures and Linux executable modes."""
import argparse
import hashlib
import json
import re
from pathlib import Path, PurePosixPath
import struct
import tarfile
import zipfile


def require(condition, message):
    if not condition:
        raise ValueError(message)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("release", type=Path, nargs="?", default=Path(__file__).resolve().parents[1] / "dist")
    args = parser.parse_args()
    release = args.release.resolve()
    versions = dict((role.lower(), version) for role, version in re.findall(r'const (Control|Node|Client)Version = "([^"]+)"', (Path(__file__).resolve().parents[1] / 'internal/model/version.go').read_text()))
    root = release
    version = versions['node']
    release = root / 'node' / version
    native = release / 'releases'
    install_manifest = json.loads((native / 'manifest.json').read_text(encoding='utf-8'))
    require(install_manifest['version'] == version and set(install_manifest['artifacts']) == {'linux/amd64','linux/arm64'}, 'native manifest version/architectures')
    for line in (native/'SHA256SUMS').read_text(encoding='ascii').splitlines():
        digest,filename=line.split()
        require(Path(filename).name==filename, 'unsafe native filename')
        require(hashlib.sha256((native/filename).read_bytes()).hexdigest()==digest, 'native checksum '+filename)
    for target,artifact in install_manifest['artifacts'].items():
        arch=target.split('/')[1]
        require(artifact['file']==f'nlroom-node-{version}-linux-{arch}.tar.gz','native archive filename')
        require(hashlib.sha256((native/artifact['file']).read_bytes()).hexdigest()==artifact['sha256'],'native manifest hash')
        with tarfile.open(native/artifact['file']) as tf:
            entries=tf.getmembers()
            require(all(i.isfile() and not PurePosixPath(i.name).is_absolute() and '..' not in PurePosixPath(i.name).parts for i in entries),'unsafe native archive')
            require(len({i.name for i in entries})==len(entries),'duplicate native archive entries')
            node=tf.getmember('nlroom-node'); require(node.mode==0o755,'native executable mode')
            require(tf.extractfile(node).read()==(release/f'nodelane-room-node-{version}-linux-{arch}'/'nlroom-node').read_bytes(),'native payload differs from full release')
            require(tf.getmember('nlroom-node.service').isfile(),'missing native systemd unit')
    print('PASS native installation manifests, two node packages and SHA256 checksums')
    for role, version in versions.items():
        release = root / role / version
        systems = (("windows", ".zip"), ("linux", ".tar.gz")) if role == 'client' else (("linux", ".tar.gz"),)
        expected = {f"nodelane-room-{role}-{version}-{system}-{arch}{extension}" for system, extension in systems for arch in ("amd64", "arm64")}
        manifest = {}
        for line in (release / "SHA256SUMS").read_text(encoding="ascii").splitlines():
            digest, name = line.split()
            require(name not in manifest, "duplicate checksum entry")
            manifest[name] = digest
        require(set(manifest) == expected, "unexpected component release archives: " + role)
        for name, digest in manifest.items():
            archive = release / name
            with archive.open("rb") as archive_file:
                hasher = hashlib.sha256()
                for chunk in iter(lambda: archive_file.read(1024 * 1024), b""):
                    hasher.update(chunk)
            require(hasher.hexdigest() == digest, f"checksum: {name}")
            arch = "arm64" if "-arm64" in name else "amd64"
            if name.endswith(".zip"):
                with zipfile.ZipFile(archive) as bundle:
                    files = {item.filename: bundle.read(item) for item in bundle.infolist() if not item.is_dir()}
                required = ["nlroom-cli.exe", "nlroom-service.exe", "nlroom-update.exe", "Install.cmd", "setup.ps1", "install.ps1", "uninstall.ps1", "NodeLaneRoom.cmd"]
                for binary in ("nlroom-cli.exe", "nlroom-service.exe", "nlroom-update.exe"):
                    data = files[binary]
                    offset = struct.unpack_from("<I", data, 0x3C)[0]
                    require(data[offset:offset + 4] == b"PE\0\0", f"invalid PE: {binary}")
                    require(struct.unpack_from("<H", data, offset + 4)[0] == {"amd64": 0x8664, "arm64": 0xAA64}[arch], f"wrong PE architecture: {binary}")
                require(not any(path.startswith("deploy/") for path in files), "server files in Windows package")
            else:
                with tarfile.open(archive, "r:gz") as bundle:
                    prefix = name.removesuffix(".tar.gz") + "/"
                    files = {}
                    for item in bundle.getmembers():
                        require(item.name == prefix[:-1] or item.name.startswith(prefix), "unexpected tar root")
                        require(item.isfile() or item.isdir(), "non-regular tar entry")
                        if item.isfile():
                            relative = item.name.removeprefix(prefix)
                            files[relative] = bundle.extractfile(item).read()
                            if relative in ("nlroom-cli", "nlroom-service", "nlroom-update", "nlroom-node", "nodelane-server"):
                                require(item.mode & 0o111 == 0o111, f"missing executable mode: {relative}")
                binaries = {'control': ['nodelane-server'], 'node': ['nlroom-node'], 'client': ['nlroom-cli', 'nlroom-service', 'nlroom-update']}[role]
                required = list(binaries)
                if role == 'control':
                    required += ['Dockerfile', '.dockerignore', 'deploy/compose.yaml', 'deploy/compose.host.yaml', 'deploy/compose.node.yaml', 'deploy/Caddyfile', 'deploy/.env.example', 'deploy/.env.host.example', 'deploy/.env.node.example']
                elif role == 'node':
                    required += ['nlroom-node.service', 'Dockerfile', '.dockerignore']
                for binary in binaries:
                    data = files[binary]
                    require(data[:4] == b"\x7fELF" and data[4:6] == b"\x02\x01", f"invalid ELF: {binary}")
                    require(struct.unpack_from("<H", data, 18)[0] == {"amd64": 62, "arm64": 183}[arch], f"wrong ELF architecture: {binary}")
                if role != "client": require("FROM golang" not in files["Dockerfile"].decode(), "release requires Go source")
                for entry in (native.iterdir() if role == "control" else []):
                    if entry.is_file(): require(files.get('releases/'+entry.name)==entry.read_bytes(),'embedded installer differs: '+entry.name)
            build = files['BUILD.txt'].decode('utf-8-sig').splitlines()
            require(f'Component: {role}' in build and f'Version: {version}' in build, 'component metadata mismatch')
            allowed = {'control': {'nodelane-server'}, 'node': {'nlroom-node'}, 'client': {'nlroom-cli', 'nlroom-service', 'nlroom-update'}}[role]
            actual = {Path(path).stem for path in files if '/' not in path and Path(path).stem in {'nodelane-server','nlroom-node','nlroom-cli','nlroom-service','nlroom-update'}}
            require(actual == allowed, 'component binary separation')
            required += ["BUILD.txt", "THIRD_PARTY_NOTICES.txt", "licenses/Go-LICENSE", "licenses/modules.txt"]
            for path in required:
                require(files.get(path), f"missing {path} in {name}")
            require(b"github.com/Wy2926/nebula v0.0.0-20260908082845-d929786cba7f" in files["BUILD.txt"], "wrong Nebula pin")
            for path, data in files.items():
                parts = PurePosixPath(path).parts
                require(".." not in parts and not PurePosixPath(path).is_absolute(), f"unsafe path: {path}")
                require(not any(part in (".local", "secrets", "__pycache__", "docs") for part in parts), f"private/test/document directory: {path}")
                require(not path.endswith((".key", ".token", "identity.bin", ".pyc", ".log")) and parts[-1] != ".env", f"private/test file: {path}")
                require(parts[-1].lower() not in ("readme.md", "agents.md"), f"source document: {path}")
                # Do not scan binaries for PEM marker strings embedded by crypto libraries.
                if path.endswith((".txt", ".ps1", ".cmd", ".yaml", ".example")):
                    require(b"-----BEGIN NEBULA X25519 PRIVATE KEY-----" not in data, f"private key: {path}")
            print(f"PASS {name}: SHA256, {arch}, {len(files)} files, required payload and no private/test artifacts")


if __name__ == "__main__":
    main()
