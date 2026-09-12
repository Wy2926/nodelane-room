"""Authenticode signing with a certificate already available to Windows SignTool."""
import argparse
from dataclasses import dataclass
import os
from pathlib import Path
import re
import shutil
import subprocess
from urllib.parse import urlsplit


@dataclass(frozen=True)
class WindowsSigner:
    tool: str
    thumbprint: str
    timestamp: str
    machine_store: bool = False

    def sign(self, path):
        args = [self.tool, 'sign', '/sha1', self.thumbprint, '/s', 'My',
                '/fd', 'SHA256', '/tr', self.timestamp, '/td', 'SHA256',
                '/d', 'NodeLane Room']
        if self.machine_store:
            args.append('/sm')
        subprocess.run(args + [str(path)], check=True)
        # Exit code 2 is also a failure: /tw warns if the timestamp is missing.
        subprocess.run([self.tool, 'verify', '/pa', '/all', '/tw', str(path)], check=True)


def load_signer(allow_unsigned=False):
    if allow_unsigned:
        return None
    thumbprint = os.environ.get('NLROOM_SIGN_CERT_SHA1', '').strip()
    if not re.fullmatch('[0-9a-fA-F]{40}', thumbprint):
        raise SystemExit('Windows release requires NLROOM_SIGN_CERT_SHA1 (certificate-store thumbprint); '
                         'use --allow-unsigned only for development packages')
    location = os.environ.get('NLROOM_SIGN_CERT_STORE_LOCATION', 'CurrentUser')
    if location not in ('CurrentUser', 'LocalMachine'):
        raise SystemExit('NLROOM_SIGN_CERT_STORE_LOCATION must be CurrentUser or LocalMachine')
    timestamp = os.environ.get('NLROOM_SIGN_TIMESTAMP_URL', '')
    parsed = urlsplit(timestamp)
    if (parsed.scheme not in ('http', 'https') or not parsed.hostname or parsed.username or parsed.password
            or parsed.query or parsed.fragment or any(character.isspace() for character in timestamp)):
        raise SystemExit('NLROOM_SIGN_TIMESTAMP_URL must be an RFC 3161 timestamp URL without credentials')
    if os.name != 'nt':
        raise SystemExit('Authenticode release signing requires Windows and the certificate private-key provider')
    tool = os.environ.get('NLROOM_SIGNTOOL_PATH') or shutil.which('signtool.exe')
    if not tool:
        sdk = Path(os.environ.get('ProgramFiles(x86)', 'C:/Program Files (x86)')) / 'Windows Kits/10/bin'
        candidates = list(sdk.glob('*/x64/signtool.exe'))
        candidates.sort(key=lambda path: tuple(int(n) for n in re.findall(r'\d+', path.parent.parent.name)))
        if candidates:
            tool = str(candidates[-1])
    if not tool or not Path(tool).is_file():
        raise SystemExit('Install Windows SDK SignTool, or set NLROOM_SIGNTOOL_PATH to signtool.exe')
    return WindowsSigner(str(Path(tool).resolve()), thumbprint.upper(), timestamp, location == 'LocalMachine')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('file', type=Path, help='Executable to sign and verify; called by the NSIS uninstaller hook')
    args = parser.parse_args()
    load_signer().sign(args.file)
