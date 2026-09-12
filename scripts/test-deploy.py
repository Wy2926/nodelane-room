#!/usr/bin/env python3
"""Smoke-test source or unpacked Linux release Compose, without publishing host ports.

Usage: python scripts/test-deploy.py [--root path/to/unpacked/linux/release]
Requires Docker Compose and a Linux Docker engine with /dev/net/tun.
"""
import argparse
import datetime
import json
import os
import re
from pathlib import Path
import secrets
import subprocess
import sys
import tempfile
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument("--node-root", type=Path, help="separate node release directory for --root release builds")
    parser.add_argument("--images", action="store_true", help="use versioned registry images instead of building")
    parser.add_argument("--host", action="store_true", help="test compose.host.yaml with isolated database/proxy fixtures (no host-port routing)")
    parser.add_argument("--pull", action="store_true", help="pull the versioned images before testing (requires --images)")
    parser.add_argument("--extended", action="store_true", help="wait for real automatic renewal and expiration; test lifecycle actions")
    versions = dict((role.lower(), version) for role, version in re.findall(r'const (Control|Node|Client)Version = "([^"]+)"', (Path(__file__).resolve().parents[1] / 'internal/model/version.go').read_text()))
    parser.add_argument("--control-version", default=versions['control'])
    parser.add_argument("--node-version", default=versions['node'])
    parser.add_argument("--registry", default="docker.nodelane.net")
    args = parser.parse_args()
    if args.pull and not args.images:
        parser.error("--pull requires --images")
    root = args.root.resolve()
    workspace = Path(__file__).resolve().parents[1]
    run = "nodelane-deploy-test-" + secrets.token_hex(6)
    out = workspace / ".local" / run
    out.mkdir(parents=True)
    password = secrets.token_hex(24)
    hidden = [password]
    env = dict(os.environ, ROOM_DOMAIN="room.test", POSTGRES_PASSWORD=password,
               NODELANE_CONTROL_VERSION=args.control_version if args.images else run,
               NODELANE_NODE_VERSION=args.node_version if args.images else run, NODELANE_REGISTRY=args.registry,
               NODE_PORT="4242", NODE_NAME="compose-test", NODE_REGION="local",
               NODE_PUBLIC_HOST="node", NODE_CONTROL_URL="https://room.test")
    env.update(CONTROL_BIND_IP="127.0.0.1")
    if args.node_root:
        env['NODELANE_NODE_BUILD_CONTEXT'] = str(args.node_root.resolve())
    checks = []

    def command(argv, *, stdin=None, log=None, check=True):
        result = subprocess.run(argv, env=env, input=stdin.encode("utf-8") if stdin is not None else None, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, timeout=1200)
        result.stdout=result.stdout.decode("utf-8",errors="replace")
        result.stderr=result.stderr.decode("utf-8",errors="replace")
        output = result.stdout + result.stderr
        for value in hidden:
            output = output.replace(value, "[REDACTED]")
        if log:
            (out / log).write_text(output, encoding="utf-8")
        if check and result.returncode:
            raise RuntimeError(f"{argv[0]} exited {result.returncode}: {output[-4000:]}")
        return result

    def passed(name):
        checks.append({"check": name, "result": "passed"})
        print("PASS " + name, flush=True)

    def config(filename):
        example = ".env.host.example" if filename == "compose.host.yaml" else ".env.example"
        argv = ["docker", "compose", "--env-file", str(root / "deploy" / example), "-f", str(root / "deploy" / filename)]
        if not args.images:
            build = "compose.build.yaml" if filename == "compose.host.yaml" else filename.replace(".yaml", ".build.yaml")
            argv += ["-f", str(root / "deploy" / build)]
        return json.loads(command(argv + ["config", "--format", "json"]).stdout)

    ok = False
    compose = None
    # This directory contains only this run's generated public config, never a CA key or node identity.
    with tempfile.TemporaryDirectory(prefix="config-", dir=out) as temporary:
        temp = Path(temporary)
        try:
            fixture = config("compose.yaml")
            control = config("compose.host.yaml") if args.host else fixture
            node = config("compose.node.yaml")
            passed("both supplied Compose files parse")
            services = control["services"]
            if args.host:
                if set(services) != {"control"} or set(control.get("volumes", {})) != {"control-state"}:
                    raise RuntimeError("host template must own one control instance and its private state volume")
                if set(services["control"].get("environment", {})) - {"NODELANE_ADMIN_PATH", "NODELANE_GEOIP_URL"}:
                    raise RuntimeError("control template still requires preconfigured application environment")
                passed("host template owns one control instance; no database, proxy or CA setup tasks")
                # Supply already-running infrastructure only in the isolated test model.
                # Host IP bindings and host-gateway reachability still require deployment verification.
                services["db"] = fixture["services"]["db"]
                services["edge"] = fixture["services"]["edge"]
                for service in services.values():
                    service["networks"] = {"backend": None}
            services["node"] = node["services"]["node"]
            services["node"]["networks"] = {"backend": None}
            # The test has no external listeners and uses Caddy's private test CA.
            for service in services.values():
                service.pop("ports", None)
                service["restart"] = "no"
                service["networks"] = {"backend": None}
            services["edge"]["networks"] = {"backend": {"aliases": ["room.test"]}}
            control["networks"] = {"backend": {"internal": True}}
            control["volumes"] = {name: {} for name in ("control-state", "node-state", "trust", "caddy-data", "caddy-config")}
            services["db"]["volumes"] = []
            services["db"]["tmpfs"] = ["/var/lib/postgresql"]
            services["node"]["environment"].update({"SSL_CERT_FILE": "/trust/root.crt"})
            services["node"]["volumes"] = [{"type":"volume","source":"node-state","target":"/var/lib/nlroom-node"}, {"type":"volume","source":"trust","target":"/trust","read_only":True}]
            caddyfile = temp / "Caddyfile"
            caddyfile.write_text((root / "deploy/Caddyfile").read_text(encoding="utf-8").replace(
                "{$ROOM_DOMAIN} {", "{$ROOM_DOMAIN} {\n  tls internal"), encoding="utf-8")
            for mount in services["edge"]["volumes"]:
                if mount["target"] == "/etc/caddy/Caddyfile":
                    mount["source"] = str(caddyfile)
            control["name"] = run
            composefile = temp / "compose.json"
            # Keep even the temporary database password out of files and logs.
            composefile.write_text(json.dumps(control).replace(password, "${POSTGRES_PASSWORD}"), encoding="utf-8")
            compose = ["docker", "compose", "-p", run, "-f", str(composefile)]
            if args.images:
                if args.pull:
                    command(compose + ["pull", "control", "node"], log="pull.log")
                passed("using published image names" + (" pulled from registry" if args.pull else " from local image store"))
            else:
                command(compose + ["build", "control", "node"], log="build.log")
                passed("control and node images build from the selected root")
            if args.host:
                command(compose + ["up", "-d", "--wait", "--wait-timeout", "120", "db"], log="database-up.log")
            command(compose + ["up", "-d", "--wait", "--wait-timeout", "180", "edge"], log="up.log")
            command(compose + ["exec", "-T", "control", "curl", "--fail", "--silent", "http://127.0.0.1:8080/healthz"])
            before = command(compose + ["exec", "-T", "control", "curl", "--fail", "--silent", "http://127.0.0.1:8080/readyz"], check=False)
            if before.returncode == 0: raise RuntimeError("unconfigured instance reported ready")
            entry = command(compose + ["exec", "-T", "control", "nodelane-server", "admin", "path"]).stdout.strip()
            hidden.append(entry)
            def check_entry():
                for path, expected in (("/", "200"), ("/product", "200"), ("/download", "200"), ("/help", "200"),
                                       ("/about", "200"), ("/privacy", "200"), ("/terms", "200"),
                                       ("/en", "200"), ("/en/product", "200"), ("/en/download", "200"), ("/en/help", "200"),
                                       ("/en/about", "200"), ("/en/privacy", "200"), ("/en/terms", "200"),
                                       ("/site-assets/site.css", "200"), ("/site-assets/", "404"),
                                       ("/admin", "404"), ("/admin/assets/app.js", "404"),
                                       (entry, "200"), (entry + "/assets/app.js", "200"), (entry + "/assets/style.css", "200")):
                    response = command(compose + ["exec", "-T", "control", "curl", "--silent", "--output", "/dev/null",
                                                  "--write-out", "%{http_code} %{redirect_url}", "http://127.0.0.1:8080" + path]).stdout.strip()
                    if response != expected: raise RuntimeError("public site or private admin entry boundary failed")
            check_entry()
            passed("single control starts without database URL or CA; public site and private setup page are reachable")
            trust = command(compose + ["exec", "-T", "edge", "cat", "/data/caddy/pki/authorities/local/root.crt"]).stdout
            command(compose + ["run", "--rm", "--no-deps", "-T", "--entrypoint", "sh", "-v", f"{run}_trust:/out",
                               "node", "-c", "cat > /out/root.crt"], stdin=trust)
            command(compose + ["up", "-d", "--wait", "--wait-timeout", "120", "node"], log="node-up.log")
            status = json.loads(command(compose+["exec","-T","node","nlroom-node","status","--json"]).stdout)
            if status["registered"] or status["engine"] != "stopped": raise RuntimeError("pending node state is incorrect")
            passed("unenrolled container stays healthy and local command is accessible")
            code = command(compose+["exec","-T","control","nodelane-server","admin","bootstrap"]).stdout.strip()
            admin_password=secrets.token_hex(24)
            hidden.extend([code,admin_password])
            csrf=''
            def admin(path,body=None,method='POST'):
                deadline=(datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(minutes=50)).isoformat().replace('+00:00','Z')
                config=['header = \"X-NodeLane-Contract: interaction-1\"','header = '+json.dumps('X-NodeLane-Operation-Deadline: '+deadline),'url = '+json.dumps('https://room.test/v2/admin/'+path), 'request = '+json.dumps(method), 'cookie = "/tmp/admin.cookies"','cookie-jar = "/tmp/admin.cookies"', 'header = "Content-Type: application/json"','header = "Origin: https://room.test"','header = '+json.dumps('X-CSRF-Token: '+csrf),'header = '+json.dumps('Idempotency-Key: '+secrets.token_hex(16))]
                if body is not None: config.append('data = '+json.dumps(json.dumps(body)))
                reply=command(compose+["exec","-T","node","curl","--fail","--silent","--show-error","--config","-"],stdin='\n'.join(config))
                result=json.loads(reply.stdout)
                if result.get("contract")!="interaction-1" or not result.get("request_id"): raise RuntimeError("invalid response contract")
                return result["data"]
            admin('setup',{'mode':'create','code':code,'username':'test-admin','password':admin_password,'database_url':f'postgres://nodelane:{password}@db:5432/nodelane?sslmode=disable','public_url':'room.test','network':'10.203.0.0/16','registry':args.registry,'ca_mode':'generate'})
            for attempt in range(30):
                if admin('setup',method='GET')['initialized']: break
                time.sleep(1)
            else: raise RuntimeError('control did not load persisted configuration')
            csrf=admin('login',{'username':'test-admin','password':admin_password})['csrf'];hidden.append(csrf)
            node_record=admin('nodes',{'name':'compose-test','region':'local','address':'node:4242','lighthouse':True,'relay':True})
            token=admin('nodes/'+node_record['id']+'/key',{})['key'];hidden.append(token)
            command(compose+["exec","-T","node","nlroom-node","enroll","--key-stdin"],stdin=token,log="enroll.log")
            passed("web database/CA setup, login, pre-created node and one-use enrollment over HTTPS")
            command(compose+["exec","-T","node","curl","--fail","--silent","http://127.0.0.1:9090/readyz"])
            manifest=json.loads(command(compose+["exec","-T","node","curl","--fail","--silent","https://room.test/install/manifest.json"]).stdout)
            if manifest['version']!=args.node_version or set(manifest['artifacts'])!={'linux/amd64','linux/arm64'}: raise RuntimeError('native release manifest mismatch')
            script=command(compose+["exec","-T","node","curl","--fail","--silent","https://room.test/install/node.sh"]).stdout
            denied=command(compose+["exec","-T","node","bash","-s","--","--server","https://room.test"],stdin=script,check=False)
            if denied.returncode==0 or '容器内' not in denied.stderr: raise RuntimeError('native installer did not reject container: '+denied.stdout+denied.stderr)
            passed("immutable native releases served and curl installer rejects containers")
            node_id = command(compose + ["ps", "-q", "node"]).stdout.strip()
            inspect = json.loads(command(["docker", "inspect", node_id]).stdout)[0]
            if inspect["HostConfig"].get("PortBindings"):
                raise RuntimeError("test unexpectedly published a host port")
            command(compose + ["exec", "-T", "node", "sh", "-c", "test -d /sys/class/net/nodelane0"])
            identity = command(compose + ["exec", "-T", "node", "sha256sum", "/var/lib/nlroom-node/identity.bin"]).stdout
            command(compose + ["up", "-d", "--force-recreate", "--wait", "--wait-timeout", "120", "node"], log="node-recreate.log")
            for attempt in range(60):
                ready=command(compose+["exec","-T","node","curl","--fail","--silent","http://127.0.0.1:9090/readyz"],check=False)
                if ready.returncode==0: break
                time.sleep(1)
            if ready.returncode!=0: raise RuntimeError('node data plane did not recover')
            again = command(compose + ["exec", "-T", "node", "sha256sum", "/var/lib/nlroom-node/identity.bin"]).stdout
            if identity != again:
                raise RuntimeError("node identity changed during container recreation")
            passed("node identity persists and TUN recovers after container recreation")
            # The HTTP test cookie jar lives in /tmp and is intentionally lost on recreation.
            csrf=admin('login',{'username':'test-admin','password':admin_password})['csrf'];hidden.append(csrf)
            def node_status():
                return json.loads(command(compose+["exec","-T","node","nlroom-node","status","--json"]).stdout)
            def eventually(description, predicate, seconds=75):
                deadline=time.monotonic()+seconds
                while time.monotonic()<deadline:
                    if predicate(): return
                    time.sleep(2)
                raise RuntimeError('timed out: '+description)
            def action(kind):
                return admin('nodes/'+node_record['id']+'/actions',{'action':kind,'expected_revision':node_status()['node']['revision']})
            existing=node_status()
            new_config={'name':'compose-test','region':'local','address':'node:4244','lighthouse':True,'relay':True,'notes':'port validation'}
            changed=admin('nodes/'+node_record['id'],{'config':new_config,'revision':existing['node']['revision']},'PUT')
            eventually('receive desired UDP configuration',lambda:node_status()['node']['revision']==changed['revision'])
            pending=node_status()
            if pending['report']['applied_revision']==changed['revision'] or pending['engine']!='running': raise RuntimeError('unapproved port did not preserve old running configuration')
            rejected=command(compose+["exec","-T","node","nlroom-node","config","apply",str(changed['revision'])],check=False)
            if rejected.returncode==0: raise RuntimeError('unmapped container port accepted')
            services['node']['command']=['run','--server','https://room.test','--listen-port','4244']
            services['node']['environment']['NLROOM_MAPPED_PORT']='4244'
            composefile.write_text(json.dumps(control).replace(password, '${POSTGRES_PASSWORD}'),encoding='utf-8')
            command(compose+["up","-d","--force-recreate","--wait","--wait-timeout","120","node"],log='port-recreate.log')
            eventually('port configuration received after recreation',lambda:node_status()['node']['revision']==changed['revision'])
            rejected=command(compose+["exec","-T","node","nlroom-node","config","apply",str(changed['revision']-1)],check=False)
            if rejected.returncode==0: raise RuntimeError('stale config revision accepted')
            command(compose+["exec","-T","node","nlroom-node","config","apply",str(changed['revision'])])
            eventually('new port applied',lambda:node_status()['report']['applied_revision']==changed['revision'] and node_status()['engine']=='running')
            csrf=admin('login',{'username':'test-admin','password':admin_password})['csrf'];hidden.append(csrf)
            passed('UDP configuration preserves old port, rejects absent mapping/stale version and applies after explicit local confirmation')
            device_id=node_status()['device_id']
            command(compose+["stop","node"])
            # Simulate loss of only the final enrollment success marker, retaining the private identity.
            edit='sed -E -i \'s/"node_id":"[0-9a-f]+"/"node_id":""/;s/"generation":[0-9]+/"generation":0/\' /var/lib/nlroom-node/identity.bin'
            command(compose+["run","--rm","--no-deps","-T","--entrypoint","sh","node","-c",edit])
            command(compose+["start","node"])
            eventually('management available after marker loss',lambda:command(compose+["exec","-T","node","nlroom-node","status","--json"],check=False).returncode==0)
            command(compose+["exec","-T","node","nlroom-node","enroll"])
            if node_status()['device_id']!=device_id: raise RuntimeError('recovery replaced private identity')
            passed('lost local enrollment marker recovers the same committed identity without a second key')
            csrf=admin('login',{'username':'test-admin','password':admin_password})['csrf'];hidden.append(csrf)
            for kind in ('drain','resume','disable','resume','restart'):
                operation=action(kind)
                eventually(kind+' acknowledgement',lambda:any(o['id']==operation['id'] and o['state']=='succeeded' for o in admin('snapshot',method='GET')['operations']))
                state=node_status()
                if state['engine']!=('stopped' if kind=='disable' else 'running'): raise RuntimeError('lifecycle engine state mismatch: '+kind)
            passed('drain, resume, disable, restore and restart acknowledge actual node execution')
            if args.extended:
                before=node_status()['lease_expires_at']
                tun_index=command(compose+["exec","-T","node","cat","/sys/class/net/nodelane0/ifindex"]).stdout
                print('Waiting for the real <=10 minute certificate renewal...',flush=True)
                eventually('automatic certificate renewal',lambda:node_status()['lease_expires_at']!=before,240)
                if command(compose+["exec","-T","node","cat","/sys/class/net/nodelane0/ifindex"]).stdout!=tun_index: raise RuntimeError('ordinary renewal restarted TUN')
                passed('real automatic Nebula renewal preserves the TUN device')
                expires=datetime.datetime.fromisoformat(node_status()['lease_expires_at'].replace('Z','+00:00'))
                command(compose+["stop","control"],log='control-stopped.log')
                print('Waiting for the issued certificate to expire with control offline...',flush=True)
                def expired():
                    return command(compose+["exec","-T","node","curl","--fail","--silent","http://127.0.0.1:9090/readyz"],check=False).returncode!=0
                eventually('data plane expires offline',expired,max(1,(expires-datetime.datetime.now(datetime.timezone.utc)).total_seconds())+40)
                if command(compose+["exec","-T","node","sh","-c","test -d /sys/class/net/nodelane0"],check=False).returncode==0: raise RuntimeError('expired node retained TUN')
                command(compose+["start","control"])
                eventually('control restoration and fresh certificate',lambda:node_status()['engine']=='running')
                passed('real certificate expiry stops offline TUN and control recovery reacquires authorization')
            deployment_id=admin('snapshot',method='GET')['deployment_id']
            command(compose + ["up", "-d", "--force-recreate", "--wait", "--wait-timeout", "120", "control"], log="control-recreate.log")
            restored_entry = command(compose + ["exec", "-T", "control", "nodelane-server", "admin", "path"]).stdout.strip()
            if restored_entry != entry: raise RuntimeError("private admin entry changed after recreation")
            check_entry()
            passed("random admin entry and assets persist after recreation; public routes stay 404 without redirects")
            eventually('persisted control configuration loads after recreation',lambda:admin('setup',method='GET')['initialized'])
            if admin('snapshot',method='GET')['deployment_id'] != deployment_id: raise RuntimeError('control identity changed after recreation')
            metrics = command(compose + ["exec", "-T", "node", "curl", "--silent", "--output", "/dev/null", "--write-out", "%{http_code}", "https://room.test/metrics"]).stdout
            if metrics != "404":
                raise RuntimeError("public metrics path was not blocked")
            passed("single control recovers configuration, CA and admin session after recreation; public metrics blocked")
            action('revoke')
            eventually('permanent revocation',lambda:node_status()['engine']=='stopped')
            denied=command(compose+["exec","-T","node","nlroom-node","enroll"],check=False)
            if denied.returncode==0: raise RuntimeError('revoked identity recovered enrollment')
            passed('permanent revocation stops data plane and rejects identity authentication')
            ok = True
        except (RuntimeError, OSError, ValueError, subprocess.SubprocessError) as exc:
            message = str(exc)
            for value in hidden:
                message = message.replace(value, "[REDACTED]")
            checks.append({"check": "deployment smoke", "result": "failed", "error": message})
            print("FAIL " + message, flush=True)
        finally:
            if compose:
                command(compose + ["logs", "--no-color", "--tail", "80"], log="services.log", check=False)
                result = command(compose + ["down", "--volumes", "--remove-orphans"], log="cleanup.log", check=False)
                ok = ok and result.returncode == 0
                checks.append({"check": "containers, test database, identities, CA and networks removed", "result": "passed" if result.returncode == 0 else "failed"})
                if result.returncode == 0 and not args.images:
                    candidates = {f"{args.registry}/nodelane-room-control:{run}", f"{args.registry}/nodelane-room-node:{run}"}
                    present = set(command(["docker", "image", "ls", "--format", "{{.Repository}}:{{.Tag}}"]).stdout.splitlines())
                    images = sorted(candidates & present)
                    if images:
                        removed = command(["docker", "image", "rm", *images], log="cleanup-images.log", check=False)
                        ok = ok and removed.returncode == 0
                        checks.append({"check": "per-run test images removed", "result": "passed" if removed.returncode == 0 else "failed"})
            (out / "results.json").write_text(json.dumps(checks, ensure_ascii=False, indent=2), encoding="utf-8")
    print("Results: " + str(out), flush=True)
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
