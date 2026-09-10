"""Run installed Linux GUI against disposable HTTPS/PostgreSQL/Nebula infrastructure."""
import argparse
import importlib.util
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('docker_test', ROOT / 'scripts/test-docker.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--package', type=Path, required=True)
    parser.add_argument('--image', default='nodelane-desktop-build:local')
    args = parser.parse_args()
    package = args.package.resolve().relative_to(ROOT).as_posix()
    run = module.TestRun()
    name = run.name + '-desktop'
    ok = False
    try:
        run.setup()
        run.rpc('bob', 'init', server='https://control:8443', name='Bob')
        created = run.rpc('bob', 'create', body={'name': 'Native GUI guest test', 'game': 'custom'})
        invitation = run.invite(created['invitation'])
        run.eventually('peer online', lambda: run.connected('bob'))
        output = run.out / 'desktop'
        output.mkdir()
        command = ['docker', 'run', '--rm', '--name', name, '--network', run.name + '_player-a',
            '--cap-add', 'NET_ADMIN', '--device', '/dev/net/tun:/dev/net/tun',
            '--sysctl', 'net.ipv4.ip_forward=0', '--env', 'SSL_CERT_FILE=/trust/control.crt',
            '--volume', run.name + '_trust:/trust:ro', '--volume', str(ROOT) + ':/workspace:ro',
            '--volume', str(output) + ':/output', '-i', args.image,
            'bash', '/workspace/scripts/desktop/test-linux.sh', '/workspace/' + package]
        result = subprocess.run(command, input=json.dumps({'invitation': invitation}),
            capture_output=True, text=True, encoding='utf-8', errors='replace', timeout=600)
        (run.out / 'desktop.log').write_text(run.redact(result.stdout + result.stderr), encoding='utf-8')
        if result.returncode: raise RuntimeError('Native desktop check failed; see desktop.log')
        run.passed('installed Linux WebView and deb lifecycle against real local service and control plane')
        ok = True
    except (RuntimeError, OSError, subprocess.SubprocessError) as exc:
        print('FAIL ' + run.redact(str(exc)))
    finally:
        subprocess.run(['docker', 'rm', '-f', name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        ok = run.finish() and ok
    return 0 if ok else 1


if __name__ == '__main__': sys.exit(main())
