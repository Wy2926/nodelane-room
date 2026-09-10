"""Assemble fresh desktop installers from explicitly selected built binaries."""
import argparse
import hashlib
from pathlib import Path
import shutil
import subprocess
import tempfile
import struct

from licenses import collect

ROOT = Path(__file__).resolve().parents[2]

def verify_binary(path, platform, arch):
    with path.open('rb') as stream:
        header = stream.read(64)
        if platform == 'windows':
            if header[:2] != b'MZ': raise SystemExit('Expected PE executable')
            stream.seek(struct.unpack_from('<I', header, 60)[0])
            pe = stream.read(6)
            if pe[:4] != b'PE\0\0': raise SystemExit('Invalid PE executable')
            machine = struct.unpack_from('<H', pe, 4)[0]
            expected = {'amd64': 0x8664, 'arm64': 0xaa64}[arch]
        else:
            if header[:6] != b'\x7fELF\x02\x01': raise SystemExit('Expected 64-bit ELF executable')
            machine = struct.unpack_from('<H', header, 18)[0]
            expected = {'amd64': 62, 'arm64': 183}[arch]
        if machine != expected: raise SystemExit('GUI architecture mismatch')

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
    import json
    version = json.loads((ROOT / 'desktop/package.json').read_text(encoding='utf-8'))['version']
    extension = '.exe' if args.platform == 'windows' else ''
    required = ['nlroom-cli' + extension, 'nlroom-service' + extension, 'BUILD.txt', 'THIRD_PARTY_NOTICES.txt']
    for name in required:
        if not (args.release / name).is_file(): raise SystemExit(f'Missing release file: {name}')
    if f'Target: {args.platform}/{args.arch}' not in (args.release / 'BUILD.txt').read_text(encoding='utf-8-sig'):
        raise SystemExit('Release architecture mismatch')
    if not args.gui.is_file(): raise SystemExit('GUI binary not found')
    verify_binary(args.gui, args.platform, args.arch)
    target = ('x86_64' if args.arch == 'amd64' else 'aarch64') + ('-pc-windows-msvc' if args.platform == 'windows' else '-unknown-linux-gnu')
    with tempfile.TemporaryDirectory(prefix='desktop-package-', dir=temp_root) as tmp:
        stage = Path(tmp)
        if args.platform == 'windows':
            payload = stage / 'payload'
            payload.mkdir()
            for name in required + ['install.ps1', 'uninstall.ps1', 'setup.ps1', 'NodeLaneRoom.cmd']:
                shutil.copy2(args.release / name, payload / name)
            shutil.copytree(args.release / 'licenses', payload / 'licenses')
            collect(ROOT, payload / 'licenses/desktop', target)
            shutil.copytree(args.release / 'dist' / 'windows' / 'wintun', payload / 'dist' / 'windows' / 'wintun')
            shutil.copy2(args.gui, payload / 'nlroom.exe')
            out = dest / f'nlroom-{version}-windows-{args.arch}-setup.exe'
            subprocess.run(['makensis', f'-DPAYLOAD={payload}', f'-DOUTPUT={out}', str(ROOT / 'scripts/desktop/windows.nsi')], check=True)
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
            shutil.copytree(args.release / 'licenses', stage / 'usr/share/doc/nlroom/licenses')
            collect(ROOT, stage / 'usr/share/doc/nlroom/licenses/desktop', target)
            applications = stage / 'usr/share/applications'
            applications.mkdir(parents=True)
            (applications / 'net.nodelane.room.desktop').write_text('[Desktop Entry]\nType=Application\nName=NodeLane Room\nComment=游戏房间联机\nExec=nlroom\nIcon=net.nodelane.room\nTerminal=false\nCategories=Game;Network;\n', encoding='utf-8')
            control = stage / 'DEBIAN'
            control.mkdir()
            (control / 'control').write_text(f'Package: nlroom\nVersion: {version}\nSection: games\nPriority: optional\nArchitecture: {args.arch}\nMaintainer: NodeLane\nDepends: libwebkit2gtk-4.1-0, libgtk-3-0, libayatana-appindicator3-1, librsvg2-2, libxdo3, systemd\nDescription: NodeLane Room desktop game networking\n', encoding='utf-8')
            scripts = {
                'postinst': 'if [ -d /run/systemd/system ]; then systemctl daemon-reload; fi\necho "Configure the installation user: sudo nlroom-setup --owner <player-user>"\n',
                'prerm': 'if [ -d /run/systemd/system ]; then systemctl stop nlroom-service.service; systemctl disable nlroom-service.service || true; fi\n',
                'postrm': 'if [ -d /run/systemd/system ]; then systemctl daemon-reload; fi\necho "Identity and owner binding retained in /var/lib/nlroom and /etc/nlroom."\n',
            }
            for name, text in scripts.items():
                (control / name).write_text('#!/bin/sh\nset -eu\n' + text, encoding='utf-8')
                (control / name).chmod(0o755)
            out = dest / f'nlroom_{version}_{args.arch}.deb'
            subprocess.run(['dpkg-deb', '--root-owner-group', '--build', str(stage), str(out)], check=True)
    digest = hashlib.sha256()
    with out.open('rb') as stream:
        for block in iter(lambda: stream.read(1 << 20), b''): digest.update(block)
    out.with_suffix(out.suffix + '.sha256').write_text(f'{digest.hexdigest()}  {out.name}\n', encoding='ascii')
    print(out)

if __name__ == '__main__':
    main()
