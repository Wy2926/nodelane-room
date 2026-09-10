"""Collect dependency notices from the exact resolved desktop sources."""
import json
from pathlib import Path
import shutil
import subprocess


def copy_notices(source, destination):
    destination.mkdir(parents=True, exist_ok=True)
    found = False
    for item in source.iterdir():
        if item.name.upper().startswith(('LICENSE', 'LICENCE', 'COPYING', 'NOTICE', 'PATENTS')):
            if item.is_dir():
                shutil.copytree(item, destination / item.name, dirs_exist_ok=True)
            else:
                shutil.copy2(item, destination / item.name)
            found = True
    if not found:
        for item in source.glob('README*'):
            if item.is_file(): shutil.copy2(item, destination / item.name)


def collect(root, destination, target):
    destination.mkdir(parents=True, exist_ok=True)
    metadata = json.loads(subprocess.check_output([
        'cargo', 'metadata', '--format-version', '1', '--locked',
        '--manifest-path', str(root / 'desktop/src-tauri/Cargo.toml'),
        '--filter-platform', target,
    ], text=True, encoding='utf-8'))
    notices = ['NodeLane Room desktop dependencies', 'Includes build-time dependencies for traceability.', '']
    for package in metadata['packages']:
        if not package['source']: continue
        name = f"{package['name']}@{package['version']}"
        notices.append(f"Rust {name}: {package['license'] or 'See bundled license'}")
        copy_notices(Path(package['manifest_path']).parent, destination / 'rust' / name)
    lock = json.loads((root / 'desktop/package-lock.json').read_text(encoding='utf-8'))
    for key, package in lock['packages'].items():
        if not key or package.get('dev'): continue
        name = key.removeprefix('node_modules/')
        folder = name.replace('/', '_') + '@' + package['version']
        notices.append(f"JavaScript {name}@{package['version']}: {package.get('license', 'See bundled license')}")
        copy_notices(root / 'desktop' / key, destination / 'javascript' / folder)
    (destination / 'DESKTOP-NOTICES.txt').write_text('\n'.join(notices) + '\n', encoding='utf-8')
