#!/usr/bin/env python3
"""Package only the reviewed image deployment templates, never live .env or keys."""
import hashlib
import re
from pathlib import Path
import zipfile

root = Path(__file__).resolve().parents[1]
versions = dict((role.lower(), version) for role, version in re.findall(r'const (Control|Node)Version = "([^"]+)"', (root / 'internal/model/version.go').read_text()))
output = root / "dist" / "compose"
output.mkdir(parents=True, exist_ok=True)
name = f"nodelane-room-compose-control-{versions['control']}-node-{versions['node']}.zip"
files = ("compose.yaml", "compose.host.yaml", "compose.network.yaml", "compose.node.yaml", ".env.example", ".env.host.example", ".env.node.example", "Caddyfile", "IMAGES.txt", "QUICKSTART.txt")
with zipfile.ZipFile(output / name, "w", zipfile.ZIP_DEFLATED) as archive:
    for filename in files:
        archive.write(root / "deploy" / filename, filename)
with zipfile.ZipFile(output / name) as archive:
    if set(archive.namelist()) != set(files) or archive.testzip() is not None:
        raise ValueError("Compose archive verification failed")
digest = hashlib.sha256((output / name).read_bytes()).hexdigest()
(output / (name + ".sha256")).write_text(f"{digest}  {name}\n", encoding="ascii")
print(output / name)
print("SHA256 " + digest)
