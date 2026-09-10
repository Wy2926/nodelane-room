"""Assemble complete desktop installers with matching versions and payload hashes."""
import argparse
import hashlib
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
import tempfile
import struct
import urllib.request

from licenses import collect

ROOT = Path(__file__).resolve().parents[2]


def source_version(root=ROOT):
    version = json.loads((root / 'desktop/package.json').read_text(encoding='utf-8'))['version']
    if not re.fullmatch(r'\d+\.\d+\.\d+', version): raise SystemExit('Use a numeric desktop release version')
    tauri = json.loads((root / 'desktop/src-tauri/tauri.conf.json').read_text(encoding='utf-8'))['version']
    cargo = re.search(r'(?m)^version = "([^"]+)"', (root / 'desktop/src-tauri/Cargo.toml').read_text())[1]
    go = re.search(r'const Version = "([^"]+)"', (root / 'internal/model/node.go').read_text())[1]
    if any(v != version for v in (tauri, cargo, go)): raise SystemExit('GUI, Rust and Go source versions must match')
    return version


def verify_release(release, gui, platform, arch, version):
    build = (release / 'BUILD.txt').read_text(encoding='utf-8-sig')
    if not re.search(rf'(?m)^Target: {platform}/{arch}\s*$', build): raise SystemExit('Release architecture mismatch')
    if not re.search(rf'(?m)^Version: {re.escape(version)}\s*$', build): raise SystemExit('Release version mismatch')
    extension = '.exe' if platform == 'windows' else ''
    for binary in [gui, release / ('nlroom-cli' + extension), release / ('nlroom-service' + extension)]:
        verify_binary(binary, platform, arch)


def payload_hashes(payload):
    lines = []
    for path in sorted(payload.rglob('*')):
        if path.is_symlink(): raise SystemExit('Payload must not contain symbolic links')
        if path.is_file() and path.name != 'PAYLOAD.sha256':
            lines.append(f'{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.relative_to(payload).as_posix()}')
    (payload / 'PAYLOAD.sha256').write_text('\n'.join(lines) + '\n', encoding='ascii')

def verify_binary(path, platform, arch):
    with path.open('rb') as stream:
        header = stream.read(64)
        if platform == 'windows':
            if len(header) < 64 or header[:2] != b'MZ': raise SystemExit('Expected PE executable')
            stream.seek(struct.unpack_from('<I', header, 60)[0])
            pe = stream.read(6)
            if len(pe) < 6 or pe[:4] != b'PE\0\0': raise SystemExit('Invalid PE executable')
            machine = struct.unpack_from('<H', pe, 4)[0]
            expected = {'amd64': 0x8664, 'arm64': 0xaa64}[arch]
        else:
            if len(header) < 64 or header[:6] != b'\x7fELF\x02\x01': raise SystemExit('Expected 64-bit ELF executable')
            machine = struct.unpack_from('<H', header, 18)[0]
            expected = {'amd64': 62, 'arm64': 183}[arch]
        if machine != expected: raise SystemExit(f'Binary architecture mismatch: {path.name}')

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--platform', choices=['windows', 'linux'], required=True)
    parser.add_argument('--arch', choices=['amd64', 'arm64'], default='amd64')
    parser.add_argument('--gui', type=Path, required=True)
    parser.add_argument('--release', type=Path, required=True, help='Matching Go release directory')
    args = parser.parse_args()
    dest = ROOT / 'dist' / 'desktop'
    dest.mkdir(parents=True, exist_ok=True)
    temp_root = ROOT / '.local'
    temp_root.mkdir(exist_ok=True)
    version = source_version()
    extension = '.exe' if args.platform == 'windows' else ''
    required = ['nlroom-cli' + extension, 'nlroom-service' + extension, 'BUILD.txt', 'THIRD_PARTY_NOTICES.txt']
    for name in required:
        if not (args.release / name).is_file(): raise SystemExit(f'Missing release file: {name}')
    verify_release(args.release, args.gui, args.platform, args.arch, version)
    metadata = json.loads(args.gui.with_suffix(args.gui.suffix + '.build.json').read_text(encoding='utf-8'))
    if metadata != {'version': version, 'platform': args.platform, 'arch': args.arch, 'sha256': hashlib.sha256(args.gui.read_bytes()).hexdigest()}:
        raise SystemExit('GUI build metadata mismatch; rebuild with scripts/desktop/build.py')
    target = ('x86_64' if args.arch == 'amd64' else 'aarch64') + ('-pc-windows-msvc' if args.platform == 'windows' else '-unknown-linux-gnu')
    with tempfile.TemporaryDirectory(prefix='desktop-package-', dir=temp_root) as tmp:
        stage = Path(tmp)
        if args.platform == 'windows':
            payload = stage / 'payload'
            payload.mkdir()
            for name in required:
                shutil.copy2(args.release / name, payload / name)
            for name in ['install.ps1', 'uninstall.ps1', 'setup.ps1', 'NodeLaneRoom.cmd']:
                shutil.copy2(ROOT / 'scripts' / name, payload / name)
            shutil.copytree(args.release / 'licenses', payload / 'licenses')
            collect(ROOT, payload / 'licenses/desktop', target)
            shutil.copytree(args.release / 'dist' / 'windows' / 'wintun', payload / 'dist' / 'windows' / 'wintun')
            shutil.copy2(args.gui, payload / 'nlroom.exe')
            # Official Evergreen bootstrapper; the elevated installer verifies
            # Microsoft's Authenticode signature before executing it.
            with urllib.request.urlopen('https://go.microsoft.com/fwlink/p/?LinkId=2124703', timeout=60) as response:
                bootstrapper = response.read((5 << 20) + 1)
            if len(bootstrapper) > 5 << 20 or not bootstrapper.startswith(b'MZ'):
                raise SystemExit('Invalid Microsoft WebView2 bootstrapper download')
            (payload / 'MicrosoftEdgeWebview2Setup.exe').write_bytes(bootstrapper)
            payload_hashes(payload)
            out = dest / f'nlroom-{version}-windows-{args.arch}-setup.exe'
            partial = out.with_suffix(out.suffix + '.partial')
            subprocess.run(['makensis', f'-DPAYLOAD={payload}', f'-DOUTPUT={partial}', str(ROOT / 'scripts/desktop/windows.nsi')], check=True)
        else:
            def copy(source, relative, mode=0o644):
                target = stage / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, target)
                target.chmod(mode)
            copy(args.gui, 'usr/bin/nlroom', 0o755)
            copy(args.release / 'nlroom-cli', 'usr/bin/nlroom-cli', 0o755)
            copy(args.release / 'nlroom-service', 'usr/lib/nlroom/nlroom-service', 0o755)
            copy(ROOT / 'deploy/nlroom-service.service', 'lib/systemd/system/nlroom-service.service')
            copy(ROOT / 'scripts/desktop/linux-setup.sh', 'usr/sbin/nlroom-setup', 0o755)
            copy(ROOT / 'desktop/src-tauri/icons/icon.png', 'usr/share/icons/hicolor/128x128/apps/net.nodelane.room.png')
            copy(args.release / 'THIRD_PARTY_NOTICES.txt', 'usr/share/doc/nlroom/THIRD_PARTY_NOTICES.txt')
            (stage / 'usr/share/nlroom').mkdir(parents=True)
            (stage / 'usr/share/nlroom/VERSION').write_text(version + '\n', encoding='ascii')
            shutil.copytree(args.release / 'licenses', stage / 'usr/share/doc/nlroom/licenses')
            collect(ROOT, stage / 'usr/share/doc/nlroom/licenses/desktop', target)
            applications = stage / 'usr/share/applications'
            applications.mkdir(parents=True)
            (applications / 'net.nodelane.room.desktop').write_text('[Desktop Entry]\nType=Application\nName=NodeLane Room\nComment=游戏房间联机\nExec=nlroom\nIcon=net.nodelane.room\nTerminal=false\nCategories=Game;Network;\n', encoding='utf-8')
            control = stage / 'DEBIAN'
            control.mkdir()
            (control / 'control').write_text(f'Package: nlroom\nVersion: {version}\nSection: games\nPriority: optional\nArchitecture: {args.arch}\nMaintainer: NodeLane\nDepends: libc6 (>= 2.35), libgcc-s1, libstdc++6, libwebkit2gtk-4.1-0, libgtk-3-0, libayatana-appindicator3-1, librsvg2-2, libxdo3, systemd\nDescription: NodeLane Room desktop game networking\n', encoding='utf-8')
            for name in ['postinst', 'prerm', 'postrm']:
                copy(ROOT / f'scripts/desktop/linux-{name}.sh', 'DEBIAN/' + name, 0o755)
            out = dest / f'nlroom_{version}_{args.arch}.deb'
            partial = out.with_suffix(out.suffix + '.partial')
            subprocess.run(['dpkg-deb', '--root-owner-group', '-Zgzip', '-z6', '--build', str(stage), str(partial)], check=True)
        os.replace(partial, out)
    digest = hashlib.sha256()
    with out.open('rb') as stream:
        for block in iter(lambda: stream.read(1 << 20), b''): digest.update(block)
    out.with_suffix(out.suffix + '.sha256').write_text(f'{digest.hexdigest()}  {out.name}\n', encoding='ascii')
    print(out)

if __name__ == '__main__':
    main()
