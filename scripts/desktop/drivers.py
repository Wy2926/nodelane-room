"""Bundle pinned, unmodified OpenVPN TAP drivers and the adapter utility."""
import ctypes
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[2]
CACHE = ROOT / '.local/desktop-drivers'
DOWNLOADS = {
    'dist.win10.zip': ('https://github.com/OpenVPN/tap-windows6/releases/download/9.27.0/dist.win10.zip', '36e2609b7ceefedcb978ce5c48caf9e0e5af83423717c4e2e3c1d7ebca8f62a5'),
    'OpenVPN-2.6.22-I001-amd64.msi': ('https://build.openvpn.net/downloads/releases/OpenVPN-2.6.22-I001-amd64.msi', '1e1bb9a712990d1b2b961de7e8df3384964e4fb6f6776a100840f0d9a82ed507'),
    'OpenVPN-2.6.22-I001-arm64.msi': ('https://build.openvpn.net/downloads/releases/OpenVPN-2.6.22-I001-arm64.msi', '65b307e15e021058f39ddc8d8bf76ef03e45aad1e91005a5c907424c7c3a8ad3'),
    'tap-windows6-9.27.0.tar.gz': ('https://codeload.github.com/OpenVPN/tap-windows6/tar.gz/refs/tags/9.27.0', '5e807ab86740b654a2a63866b8749f582f16258d8e4dbcb6813be488ec59dde3'),
    'openvpn-2.6.22.tar.gz': ('https://build.openvpn.net/downloads/releases/openvpn-2.6.22.tar.gz', 'f46df740f05f86020137a41cfc8814352391cf861ed57f57b4e815cb97c1d2cf'),
}


def download(name):
    CACHE.mkdir(parents=True, exist_ok=True)
    path = CACHE / name
    url, digest = DOWNLOADS[name]
    if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != digest:
        with urllib.request.urlopen(url, timeout=60) as response:
            data = response.read((20 << 20) + 1)
        if len(data) > 20 << 20 or hashlib.sha256(data).hexdigest() != digest:
            raise SystemExit('Driver download checksum mismatch: ' + name)
        path.write_bytes(data)
    return path


def extract_tapctl(package, output):
    # Read the MSI database and cabinet without invoking any installation action.
    with tempfile.TemporaryDirectory(dir=CACHE) as tmp:
        stage = Path(tmp)
        if os.name != 'nt':
            subprocess.run(['msiextract', '-C', str(stage), str(package)], check=True, stdout=subprocess.DEVNULL)
            matches = list(stage.rglob('tapctl.exe'))
            if len(matches) != 1: raise SystemExit('Expected one tapctl executable')
            shutil.copy2(matches[0], output)
            return
        msi = ctypes.WinDLL('msi')
        handle = ctypes.c_uint

        def check(code):
            if code: raise OSError(code, 'MSI extraction failed')

        database = handle()
        check(msi.MsiOpenDatabaseW(ctypes.c_wchar_p(str(package)), None, ctypes.byref(database)))

        def records(query):
            view = handle()
            check(msi.MsiDatabaseOpenViewW(database, ctypes.c_wchar_p(query), ctypes.byref(view)))
            try:
                check(msi.MsiViewExecute(view, 0))
                while True:
                    record = handle()
                    code = msi.MsiViewFetch(view, ctypes.byref(record))
                    if code == 259: break
                    check(code)
                    try: yield record
                    finally: msi.MsiCloseHandle(record)
            finally: msi.MsiCloseHandle(view)

        def value(record, field):
            buffer, size = ctypes.create_unicode_buffer(4096), ctypes.c_uint(4096)
            check(msi.MsiRecordGetStringW(record, field, buffer, ctypes.byref(size)))
            return buffer.value

        try:
            files = [(value(r, 1), int(value(r, 3))) for r in records('SELECT `File`, `FileName`, `Sequence` FROM `File`') if value(r, 2).split('|')[-1].lower() == 'tapctl.exe']
            if len(files) != 1: raise SystemExit('Expected one tapctl executable')
            identifier, sequence = files[0]
            if Path(identifier).name != identifier: raise SystemExit('Invalid MSI file identifier')
            media = sorted((int(value(r, 1)), value(r, 2)) for r in records('SELECT `LastSequence`, `Cabinet` FROM `Media`'))
            cabinet = next(name for last, name in media if sequence <= last)
            if not cabinet.startswith('#'): raise SystemExit('Expected an embedded MSI cabinet')
            cabinet_path = stage / 'payload.cab'
            found = False
            for record in records('SELECT `Name`, `Data` FROM `_Streams`'):
                if value(record, 1) != cabinet[1:]: continue
                found = True
                with cabinet_path.open('wb') as stream:
                    while True:
                        buffer, size = ctypes.create_string_buffer(65536), ctypes.c_uint(65536)
                        check(msi.MsiRecordReadStream(record, 2, buffer, ctypes.byref(size)))
                        if not size.value: break
                        stream.write(buffer.raw[:size.value])
            if not found: raise SystemExit('Missing MSI cabinet')
            subprocess.run([str(Path(os.environ['SystemRoot']) / 'System32/expand.exe'), '-F:' + identifier, str(cabinet_path), str(stage)], check=True, stdout=subprocess.DEVNULL)
            shutil.copy2(stage / identifier, output)
        finally: msi.MsiCloseHandle(database)


def bundle_tap(engine, licenses, arch):
    engine.mkdir(parents=True)
    with zipfile.ZipFile(download('dist.win10.zip')) as archive:
        for name in ('OemVista.inf', 'tap0901.cat', 'tap0901.sys'):
            (engine / name).write_bytes(archive.read(f'dist.win10/{arch}/{name}'))
    extract_tapctl(download(f'OpenVPN-2.6.22-I001-{arch}.msi'), engine / 'tapctl.exe')
    lines = [hashlib.sha256(p.read_bytes()).hexdigest() + '  ' + p.name for p in sorted(engine.iterdir())]
    (engine / 'SHA256SUMS').write_text('\n'.join(lines) + '\n', encoding='ascii')
    # Include the complete corresponding sources and their original license texts.
    legal = licenses / 'tap-windows6'
    legal.mkdir(parents=True)
    for name in ('tap-windows6-9.27.0.tar.gz', 'openvpn-2.6.22.tar.gz'):
        shutil.copy2(download(name), legal / name)
    text = 'TAP-Windows6 9.27.0 and tapctl from OpenVPN 2.6.22 (GPL-2.0).\nUnmodified signed binaries; corresponding sources and licenses are in the adjacent archives.\n'
    text += '\n'.join(name + '\n' + url + '\nSHA256 ' + digest for name, (url, digest) in DOWNLOADS.items())
    (legal / 'SOURCES.txt').write_text(text + '\n', encoding='utf-8')
