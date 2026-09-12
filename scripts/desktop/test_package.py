"""Packaging and Debian lifecycle checks; no host service is installed."""
import hashlib
import os
import re
from pathlib import Path
import struct
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from package import ROOT, payload_hashes, source_version, verify_binary, verify_release, windows_payload


class PackagingTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        (ROOT / '.local').mkdir(exist_ok=True)

    def test_windows_installed_payload_excludes_installer_tools(self):
        with tempfile.TemporaryDirectory(dir=ROOT / '.local') as tmp:
            root = Path(tmp)
            release, stage = root / 'release', root / 'stage'
            release.mkdir()
            stage.mkdir()
            (release / 'licenses').mkdir()
            required = ['nlroom-cli.exe', 'nlroom-service.exe', 'nlroom-update.exe', 'BUILD.txt', 'THIRD_PARTY_NOTICES.txt']
            for name in required + ['nlroom.exe']:
                (release / name).write_bytes(b'fixture')
            with patch('package.collect'):
                payload, engine = windows_payload(stage, release, release / 'nlroom.exe', required, 'x86_64-pc-windows-msvc')
            self.assertEqual({p.name for p in engine.iterdir()}, {'nlroom-update.exe'})
            self.assertFalse(any(p.suffix in ('.ps1', '.cmd', '.bat') for p in payload.rglob('*')))
            self.assertFalse((payload / 'MicrosoftEdgeWebview2Setup.exe').exists())
            self.assertIn('nlroom.exe', (payload / 'PAYLOAD.sha256').read_text())

    def test_versions_are_aligned(self):
        self.assertRegex(source_version(), r'^\d+\.\d+\.\d+$')

    def test_client_version_is_independent_and_mismatch_is_rejected(self):
        with tempfile.TemporaryDirectory(dir=ROOT / '.local') as tmp:
            root = Path(tmp)
            for name in ('desktop/package.json', 'desktop/src-tauri/tauri.conf.json', 'desktop/src-tauri/Cargo.toml', 'internal/model/version.go'):
                target = root / name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text((ROOT / name).read_text(encoding='utf-8'), encoding='utf-8')
            version = source_version(root)
            go = root / 'internal/model/version.go'
            go.write_text(f'const ControlVersion = "8.1.0"\nconst NodeVersion = "9.2.0"\nconst ClientVersion = "{version}"\n')
            self.assertEqual(source_version(root), version)
            go.write_text(go.read_text().replace(f'ClientVersion = "{version}"', 'ClientVersion = "7.3.0"'))
            with self.assertRaisesRegex(SystemExit, 'client source versions must match'):
                source_version(root)

    def test_installer_languages_have_complete_matching_resources(self):
        dictionaries = []
        for locale in ('zh-CN', 'en-US'):
            source = (ROOT / f'scripts/desktop/locales/{locale}.nsh').read_text('utf-8-sig')
            entries = re.findall(r'^LangString (\w+) \$\{LANG_\w+\} "(.*)"$', source, re.M)
            self.assertEqual(len(entries), len(dict(entries)), 'Duplicate installer message')
            dictionaries.append(dict(entries))
        self.assertEqual(dictionaries[0].keys(), dictionaries[1].keys())
        for key in dictionaries[0]:
            self.assertTrue(dictionaries[1][key].strip())
            self.assertNotRegex(dictionaries[1][key], r'[\u4e00-\u9fff]')
            variables = r'\$\{\w+\}|\$[A-Za-z]\w*'
            self.assertEqual(sorted(re.findall(variables, dictionaries[0][key])), sorted(re.findall(variables, dictionaries[1][key])))
        wizard = (ROOT / 'scripts/desktop/windows.nsi').read_text('utf-8')
        self.assertNotRegex(wizard, r'[\u4e00-\u9fff]')
        self.assertEqual(set(re.findall(r'\$\((\w+)\)', wizard)), dictionaries[0].keys())

    def test_all_binaries_and_release_metadata_are_checked(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            header = bytearray(64)
            header[:6] = b'\x7fELF\x02\x01'
            struct.pack_into('<H', header, 18, 62)
            for name in ('nlroom', 'nlroom-cli', 'nlroom-service', 'nlroom-update'):
                (root / name).write_bytes(header)
            (root / 'BUILD.txt').write_text('Component: client\nVersion: 0.2.0\nTarget: linux/amd64\n')
            verify_release(root, root / 'nlroom', 'linux', 'amd64', '0.2.0')
            with self.assertRaisesRegex(SystemExit, 'version mismatch'):
                verify_release(root, root / 'nlroom', 'linux', 'amd64', '0.2.1')
            struct.pack_into('<H', header, 18, 183)
            (root / 'nlroom-service').write_bytes(header)
            with self.assertRaisesRegex(SystemExit, 'nlroom-service'):
                verify_release(root, root / 'nlroom', 'linux', 'amd64', '0.2.0')
            (root / 'nlroom').write_bytes(b'MZ')
            with self.assertRaises(SystemExit): verify_binary(root / 'nlroom', 'windows', 'amd64')

    def test_manifest_covers_nested_files_and_is_repeatable(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'licenses').mkdir()
            (root / 'licenses/NOTICE').write_text('notice')
            (root / 'nlroom').write_bytes(b'binary')
            payload_hashes(root)
            first = (root / 'PAYLOAD.sha256').read_bytes()
            payload_hashes(root)
            self.assertEqual(first, (root / 'PAYLOAD.sha256').read_bytes())
            self.assertEqual(len(first.splitlines()), 2)
            for line in first.decode().splitlines():
                digest, path = line.split('  ')
                self.assertEqual(digest, hashlib.sha256((root / path).read_bytes()).hexdigest())


@unittest.skipIf(os.name == 'nt', 'Debian lifecycle scripts execute on Linux')
class DebianLifecycleTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.log = self.root / 'calls'
        self.owner = self.root / 'owner.uid'
        self.env = dict(os.environ, PATH=str(self.root) + ':' + os.environ['PATH'], CALLS=str(self.log))
        self.command('systemctl', 'echo "systemctl $*" >> "$CALLS"\ncase "$1" in\nstop) [ "${FAIL_STOP:-0}" = 0 ];;\nshow) echo "${SERVICE_PID:-0}";;\nis-enabled) [ "${ENABLED:-1}" = 1 ];;\nis-active) [ "${UPDATE_ACTIVE:-0}" = 1 ];;\nesac\n')
        self.command('nlroom-setup', 'echo "ready $*" >> "$CALLS"\n[ "${FAIL_READY:-0}" = 0 ]\n')

    def command(self, name, body):
        path = self.root / name
        path.write_text('#!/bin/sh\nset -eu\n' + body)
        path.chmod(0o755)

    def execute(self, name, action):
        text = (ROOT / f'scripts/desktop/linux-{name}.sh').read_text()
        text = text.replace('/run/systemd/system', str(self.root)).replace('/etc/nlroom/owner.uid', str(self.owner))
        return subprocess.run(['sh', '-c', text, name, action], env=self.env, capture_output=True)

    def calls(self):
        return self.log.read_text() if self.log.exists() else ''

    def test_upgrade_stops_before_replacement_and_restarts_with_binding(self):
        self.owner.write_text('1000\n')
        self.assertEqual(self.execute('prerm', 'upgrade').returncode, 0)
        self.assertNotIn('disable', self.calls())
        self.assertEqual(self.execute('postinst', 'configure').returncode, 0)
        self.assertLess(self.calls().index('stop'), self.calls().index('start'))
        self.assertIn('ready --check', self.calls())
        self.assertEqual(self.owner.read_text(), '1000\n')

    def test_first_install_does_not_start_an_unbound_service(self):
        self.assertEqual(self.execute('postinst', 'configure').returncode, 0)
        self.assertNotIn('systemctl start', self.calls())

    def test_stop_failure_or_live_process_aborts_upgrade(self):
        for key in ('FAIL_STOP', 'SERVICE_PID'):
            with self.subTest(key=key):
                self.env[key] = '1'
                self.assertNotEqual(self.execute('prerm', 'upgrade').returncode, 0)
                del self.env[key]

    def test_failed_readiness_is_not_successful_installation(self):
        self.owner.write_text('1000\n')
        self.env['FAIL_READY'] = '1'
        self.assertNotEqual(self.execute('postinst', 'configure').returncode, 0)

    def test_abort_upgrade_recovers_enabled_service_and_remove_retains_identity(self):
        self.owner.write_text('1000\n')
        self.assertEqual(self.execute('postinst', 'abort-upgrade').returncode, 0)
        self.assertIn('systemctl start', self.calls())
        self.assertEqual(self.execute('prerm', 'remove').returncode, 0)
        self.assertIn('disable', self.calls())
        self.assertEqual(self.execute('postrm', 'purge').returncode, 0)
        self.assertTrue(self.owner.exists())

    def test_disabled_service_stays_disabled(self):
        self.owner.write_text('1000\n')
        self.env['ENABLED'] = '0'
        self.assertEqual(self.execute('postinst', 'configure').returncode, 0)
        self.assertNotIn('systemctl start', self.calls())

    def test_removal_during_update_does_not_stop_the_service(self):
        self.env['UPDATE_ACTIVE'] = '1'
        self.assertNotEqual(self.execute('prerm', 'remove').returncode, 0)
        self.assertNotIn('systemctl stop', self.calls())
        self.assertNotIn('disable', self.calls())


if __name__ == '__main__': unittest.main()
