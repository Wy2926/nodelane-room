"""Build desktop assets and a matching fresh installer; never install it."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]


def run(args, **kwargs):
    subprocess.run(args, cwd=ROOT, check=True, **kwargs)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--platform', choices=['windows', 'linux'], required=True)
    parser.add_argument('--arch', choices=['amd64', 'arm64'], default='amd64')
    parser.add_argument('--release', type=Path, required=True, help='Matching Go release directory')
    parser.add_argument('--skip-web', action='store_true', help='Use frontend assets already built and checked')
    args = parser.parse_args()
    npm = 'npm.cmd' if os.name == 'nt' else 'npm'
    if not args.skip_web:
        run([npm, '--prefix', 'desktop', 'ci'])
        run([npm, '--prefix', 'desktop', 'run', 'build'])
        run([npm, '--prefix', 'desktop', 'test'])
    if not (ROOT / 'desktop/dist/index.html').is_file(): raise SystemExit('Build frontend assets first')
    target = ('x86_64' if args.arch == 'amd64' else 'aarch64') + ('-pc-windows-msvc' if args.platform == 'windows' else '-unknown-linux-gnu')
    cargo = ['cargo']
    environment = os.environ.copy()
    if args.platform == 'windows':
        if os.name != 'nt': cargo.append('xwin')
        environment['CARGO_TARGET_' + target.upper().replace('-', '_') + '_RUSTFLAGS'] = '-C target-feature=+crt-static'
    run(cargo + ['build', '--manifest-path', 'desktop/src-tauri/Cargo.toml', '--locked', '--release', '--features', 'custom-protocol', '--target', target], env=environment)
    metadata = json.loads(subprocess.check_output(['cargo', 'metadata', '--format-version', '1', '--no-deps', '--manifest-path', str(ROOT / 'desktop/src-tauri/Cargo.toml')], encoding='utf-8'))
    extension = '.exe' if args.platform == 'windows' else ''
    built = Path(metadata['target_directory']) / target / 'release' / ('nlroom' + extension)
    output = ROOT / 'dist/desktop' / f'nlroom-{args.platform}-{args.arch}{extension}'
    output.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(built, output)
    run([sys.executable, 'scripts/desktop/package.py', '--platform', args.platform, '--arch', args.arch, '--gui', str(output), '--release', str(args.release)])


if __name__ == '__main__':
    main()
