"""Run two isolated Linux clients against disposable NodeLane infrastructure.

Requires Python 3.10+, Docker Compose and a Linux Docker engine with /dev/net/tun.
All resources are scoped to a unique Compose project and removed on exit.
"""

import argparse
import datetime
import json
import ipaddress
import os
from pathlib import Path
import secrets
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parent.parent


class TestRun:
    def __init__(self):
        self.name = "nodelane-test-" + datetime.datetime.now().strftime("%Y%m%d-%H%M%S-") + secrets.token_hex(3)
        self.out = ROOT / ".local" / self.name
        self.out.mkdir(parents=True)
        password = secrets.token_urlsafe(24)
        self.secrets = [password]
        self.env = dict(os.environ, NODELANE_TEST_RUN=self.name,
                        NODELANE_TEST_DB_PASSWORD=password)
        self.base = ["docker", "compose", "-p", self.name, "-f", str(ROOT / "deploy/test/compose.yaml")]
        self.results = []

    def redact(self, value):
        for secret in self.secrets:
            value = value.replace(secret, "[redacted]")
        return value

    def command(self, *args, input=None, timeout=90, check=True, log=None):
        try:
            result = subprocess.run([*self.base, *args], input=input, text=True,
                                    capture_output=True, cwd=ROOT, env=self.env, timeout=timeout,
                                    encoding="utf-8", errors="replace")
        except subprocess.TimeoutExpired:
            raise RuntimeError("Docker operation timed out") from None
        if log:
            (self.out / log).write_text(self.redact(result.stdout + result.stderr), encoding="utf-8")
        if check and result.returncode:
            raise RuntimeError(self.redact(result.stderr.strip() or "Docker operation failed"))
        return result

    def execute(self, service, *args, **kwargs):
        return self.command("exec", "-T", service, *args, **kwargs)

    def call(self, service, mode, req, denied=False):
        result = self.execute(service, "python3", "/opt/test/peer.py", mode,
                              input=json.dumps(req))
        reply = json.loads(result.stdout)
        if denied:
            self.require(reply["status"] == 400 and "error" in reply["data"], "operation must be rejected")
        else:
            self.require(reply["status"] == 200, "client operation failed: " + self.redact(str(reply["data"])))
        return reply["data"]

    def rpc(self, service, action, denied=False, **args):
        return self.call(service, "rpc", {"action": action, **args}, denied=denied)

    def fixture(self, service, action, **args):
        return self.call(service, "call", {"action": action, **args})

    def require(self, condition, message):
        if not condition:
            raise RuntimeError(message)

    def passed(self, name, **evidence):
        self.results.append({"check": name, "result": "passed", **evidence})
        print("PASS " + name, flush=True)

    def eventually(self, name, function, timeout=60):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            value = function()
            if value:
                return value
            time.sleep(1)
        raise RuntimeError("Timed out: " + name)

    def status(self, service):
        return self.rpc(service, "status")

    def connected(self, service):
        value = self.status(service)
        return value if value["engine"] == "running" and value["control"] == "connected" and not value.get("error") else None

    def echo(self, service, ip, protocol="tcp", port=26001):
        return self.fixture(service, "echo", ip=ip, port=port, protocol=protocol)["ok"]

    def stopped(self, service):
        self.eventually(service + " stopped", lambda: self.status(service)["engine"] == "stopped")
        for name in ("nodelane0-lan",):
            result = self.execute(service, "ip", "link", "show", name, check=False)
            self.require(result.returncode != 0, "virtual interface remains after stop")

    def invite(self, value):
        code = value["code"]
        self.secrets.append(code)
        return code

    def setup(self):
        self.command("config", "--quiet")
        print("Building the Docker test image...", flush=True)
        self.command("build", "control", timeout=1200, log="build.log")
        print("Starting the private database and HTTPS control service...", flush=True)
        self.command("up", "-d", "--wait", "--wait-timeout", "120", "db", "control", timeout=240, log="startup.log")
        code = self.execute("control", "nodelane-server", "--state-dir", "/state/control", "admin", "bootstrap").stdout.strip()
        password = secrets.token_hex(24)
        self.admin_password = password
        self.secrets.extend([code, password])
        admin = self.execute("control", "python3", "/opt/test/admin.py", input=json.dumps({"code":code,"username":"test-admin","password":password,"database_url":f"postgres://nodelane:{self.env['NODELANE_TEST_DB_PASSWORD']}@db:5432/nodelane_test?sslmode=disable","create":{"name":"test-relay","region":"docker","address":"relay:4242","lighthouse":True,"relay":True}}))
        token = json.loads(admin.stdout)['key']
        self.secrets.append(token)
        self.command("up", "-d", "relay")
        self.eventually("node management socket", lambda:self.execute("relay","nlroom-node","--state-dir","/state/node","status","--json",check=False).returncode==0)
        self.execute("relay", "nlroom-node", "--state-dir", "/state/node", "enroll", "--key-stdin", input=token)
        self.command("up", "-d", "--wait", "--wait-timeout", "90", "relay", "alice", "bob", timeout=120)
        self.passed("HTTPS control, PostgreSQL and real Nebula lighthouse/relay started")

    def isolation(self):
        networks = []
        for service, own, other in (("alice", "172.30.81.10", "172.30.82.10"),
                                     ("bob", "172.30.82.10", "172.30.81.10")):
            cid = self.command("ps", "-q", service).stdout.strip()
            raw = subprocess.run(["docker", "inspect", "--format", "{{json .NetworkSettings.Networks}}", cid],
                                 text=True, capture_output=True, check=True, timeout=15)
            networks.append(set(json.loads(raw.stdout)))
            route = self.execute(service, "ip", "route", "get", other, check=False)
            self.require(route.returncode != 0, "underlay unexpectedly routable")
            for protocol in ("tcp", "udp"):
                self.require(self.echo(service, own, protocol), "local fixture is not listening")
                self.require(not self.echo(service, other, protocol), "underlays can communicate")
        self.require(len(networks[0]) == len(networks[1]) == 1 and not networks[0] & networks[1],
                     "player containers must have separate networks")
        for service in ("control", "relay"):
            value = self.execute(service, "cat", "/proc/sys/net/ipv4/ip_forward").stdout.strip()
            self.require(value == "0", "infrastructure must not forward underlay packets")
        self.passed("disjoint player networks; bidirectional underlay TCP/UDP denied; forwarding disabled")

    def traffic(self):
        a = self.eventually("alice online", lambda: self.connected("alice"))
        b = self.eventually("bob online", lambda: self.connected("bob"))
        self.require(a["ip"] != b["ip"], "duplicate overlay address")
        for service, peer in (("alice", b), ("bob", a)):
            route = self.execute(service, "ip", "-j", "route", "get", peer["ip"])
            self.require(json.loads(route.stdout)[0]["dev"] == "nodelane0-lan", "traffic bypasses TAP")
            for protocol in ("tcp", "udp"):
                self.eventually("allowed " + protocol, lambda: self.echo(service, peer["ip"], protocol))
                self.require(not self.echo(service, peer["ip"], protocol, 26002), "unregistered port passed")
        paths = {}
        for service, peer in (("alice", b), ("bob", a)):
            self.eventually("real relay path", lambda: any(p["device_id"] == peer["device_id"] and p["mode"] == "relay"
                                                            for p in self.status(service)["peers"]))
            probes = []
            for _ in range(3):
                try:
                    probe = self.rpc(service, "ping", target=peer["device_id"])
                    self.require(probe["rtt_ms"] > 0, "invalid real probe RTT")
                    probes.append({"result": "reply", "rtt_ms": probe["rtt_ms"]})
                except RuntimeError as exc:
                    if "overlay probe timed out" not in str(exc):
                        raise
                    probes.append({"result": "timeout"})
                time.sleep(1)
            self.require(any(p["result"] == "reply" for p in probes), "all real probes timed out")
            state = next(p for p in self.status(service)["peers"] if p["device_id"] == peer["device_id"])
            self.require(state["mode"] == "relay", "relay path disappeared")
            paths[service] = {"ip": a["ip"] if service == "alice" else b["ip"],
                              "peer_mode": "relay", "probes": probes,
                              "rtt_ms": state.get("rtt_ms"), "loss_percent": state.get("loss_percent")}
        self.passed("bidirectional TAP TCP/UDP, actual relay paths, measured RTT and closed-port rejection", peers=paths)
        return a, b

    def monitoring(self, alice, bob):
        def observed():
            reply = self.execute("control", "python3", "/opt/test/admin.py", input=json.dumps({
                "username": "test-admin", "password": self.admin_password, "path": "telemetry", "method": "GET",
                "wait_telemetry": [alice["device_id"], bob["device_id"]]}), timeout=120)
            data = json.loads(reply.stdout)
            sources = {s["device_id"]: s for s in data["series"]}
            for own, peer in ((alice, bob), (bob, alice)):
                source = sources.get(own["device_id"])
                if not source or len(source["samples"]) < 7:
                    return None
                samples = source["samples"]
                at = lambda s: datetime.datetime.fromisoformat(s["at"].replace("Z", "+00:00"))
                if (at(samples[-1]) - at(samples[0])).total_seconds() < 30:
                    return None
                current = samples[-1]
                traffic = current.get("traffic", {})
                if not traffic.get("upload_bytes") or not traffic.get("download_bytes"):
                    raise RuntimeError("member traffic missing: " + json.dumps(traffic))
                link = next((p for p in current["peers"] if p["device_id"] == peer["device_id"]), {})
                if link.get("mode") != "relay" or not link.get("relay_ips") or link.get("rtt_ms") is None or link.get("loss_percent") is None:
                    raise RuntimeError("member measured relay path missing: " + json.dumps(link))
            nodes = [s for s in data["series"] if s.get("node_id")]
            if not nodes:
                return None
            sample = nodes[0]["samples"][-1]
            if sample.get("traffic", {}).get("scope") != "nebula_udp" or not sample["traffic"]["upload_bytes"] or not sample["traffic"]["download_bytes"]:
                raise RuntimeError("node forwarded traffic missing: " + json.dumps(sample.get("traffic")))
            for player in (alice, bob):
                if not any(p["device_id"] == player["device_id"] and p.get("remote") for p in sample["peers"]):
                    raise RuntimeError("node did not observe the member endpoint")
            return data
        data = observed()
        self.require(data is not None, "valid telemetry window missing after collection")
        self.passed("admin telemetry retains >=30s of real relay, RTT/loss, member and forwarded UDP traffic, observed exits",
                    reporters=len(data["series"]), retention_seconds=data["retention_seconds"])

    def configure(self, game_id, ports, discovery=True):
        game = next(g for g in self.rpc("alice", "games") if g["id"] == game_id)
        update = {"username": "test-admin", "password": self.admin_password,
                  "path": "games/" + game_id, "method": "PUT", "body": {
                      "name": game["name"], "revision": game["revision"], "enabled": True,
                      "ports": ports, "network": {"version": 1, "broadcast": discovery,
                                                   "multicast": discovery, "ethernet_types": []}}}
        self.execute("control", "python3", "/opt/test/admin.py", input=json.dumps(update))

    def business(self):
        a_id = self.rpc("alice", "init", server="https://control:8443", name="Alice")
        b_id = self.rpc("bob", "init", server="https://control:8443", name="Bob")
        self.require(a_id != b_id, "devices share an identity")
        self.configure("custom", [{"protocol": "tcp", "port": 26002}])
        created = self.rpc("alice", "create", body={"name": "Docker regression", "game": "custom"})
        room = created["room"]["id"]
        old_code = self.invite(created["invitation"])
        code = self.invite(self.rpc("alice", "invite"))
        error = self.rpc("bob", "join", body={"code": old_code}, denied=True)
        self.require("control API 403" in error["error"], "old invitation rejected for unexpected reason")
        self.rpc("bob", "join", body={"code": code})
        self.eventually("two members", lambda: len(self.rpc("alice", "members")["members"]) == 2)
        for service in ("alice", "bob"):
            self.eventually(service + " online", lambda: self.connected(service))
            cli = json.loads(self.execute(service, "nlroom-cli", "--state-dir", "/state/client", "status", "--json").stdout)
            self.require(cli["engine"] == "running" and cli["room"]["id"] == room and cli["protocol_version"] == 2,
                         "CLI status differs from joined room")
        self.rpc("bob", "create", body={"name": "Second active room", "game": "custom"}, denied=True)
        self.passed("independent registration, create/join, invitation rotation and one active room via current IPC")
        b_ip = self.status("bob")["ip"]
        for protocol in ("tcp", "udp"):
            self.require(not self.echo("alice", b_ip, protocol), "game port open before server authorization")
        self.configure("custom", [{"protocol": protocol, "port": 26001} for protocol in ("tcp", "udp")])
        a, b = self.traffic()
        self.monitoring(a, b)
        self.passed("default deny and administrator configuration apply to the generic LAN game")
        self.rpc("bob", "leave")
        self.stopped("bob")
        self.require(not self.echo("alice", b["ip"]), "traffic survives leave")
        self.rpc("bob", "join", body={"code": code})
        self.eventually("bob rejoined", lambda: self.connected("bob"))
        self.passed("leave stops TAP and traffic; valid invitation permits rejoin")
        self.rpc("bob", "leave")
        self.stopped("bob")
        self.rpc("bob", "create", body={"name": "Other Docker room", "game": "custom"})
        b = self.eventually("bob other room", lambda: self.connected("bob"))
        self.eventually("Bob left original snapshot", lambda: len(self.rpc("alice", "members")["members"]) == 1)
        for service, peer in (("alice", b), ("bob", a)):
            for protocol in ("tcp", "udp"):
                self.require(not self.echo(service, peer["ip"], protocol), "cross-room traffic passed")
        self.passed("bidirectional cross-room TCP/UDP denied despite identical game policies")
        self.rpc("bob", "close")
        self.stopped("bob")
        self.rpc("alice", "close", room=room)
        self.stopped("alice")
        self.passed("room closure removes TAP devices")

    def configured_game(self):
        games = self.rpc("alice", "games")
        self.require(any(g["id"] == "custom" for g in games), "custom game absent")
        game = next(g for g in games if g["id"] == "minecraft-java")
        created = self.rpc("alice", "create", body={"name": "Configured TCP game", "game": game["id"]})
        code = self.invite(created["invitation"])
        self.rpc("bob", "join", body={"code": code})
        for service in ("alice", "bob"):
            self.eventually(service + " configured game ready", lambda: self.connected(service))
        alice = self.status("alice")
        self.eventually("automatic server-configured TCP", lambda: self.echo("bob", alice["ip"], port=25565))
        self.require(not self.echo("bob", alice["ip"], port=26001), "unconfigured port accepted")
        self.configure(game["id"], [{"protocol": "tcp", "port": 26002}])
        self.eventually("new configured port passes", lambda: self.echo("bob", alice["ip"], port=26002))
        self.require(not self.echo("bob", alice["ip"], port=25565), "removed configured port still passes")
        self.passed("server game list, LAN game ports and live policy replacement")
        bob = self.status("bob")
        self.rpc("alice", "kick", body={"device_id": bob["device_id"]})
        self.stopped("bob")
        self.require(not self.echo("bob", alice["ip"], port=26002), "revoked game traffic survives")
        denial = self.rpc("bob", "join", body={"code": code}, denied=True)
        self.require("control API 403" in denial["error"], "kicked member rejected for unexpected reason")
        self.rpc("alice", "close")
        self.stopped("alice")
        self.passed("generic game kick removes TAP, stops traffic and denies rejoin")

    def verify(self):
        print("Running Linux vet, full tests and full race tests with the dedicated database...", flush=True)
        self.command("--profile", "verify", "build", "verify", timeout=1200, log="verify-build.log")
        failed = False
        fixture = os.environ.get("NODELANE_TEST_GEOIP_DB")
        fixture_args = ["--volume", str(Path(fixture).resolve()) + ":/tmp/GeoIP2-City-Test.mmdb:ro",
                        "--env", "NODELANE_TEST_GEOIP_DB=/tmp/GeoIP2-City-Test.mmdb"] if fixture else []
        for name, args in (("vet", ["go", "vet", "./..."]),
                           ("test", ["go", "test", "-count=1", "./..."]),
                           ("race", ["go", "test", "-race", "-count=1", "./..."])):
            result = self.command("run", "--rm", "--no-deps", *fixture_args, "verify", *args,
                                  timeout=1200, check=False, log=name + ".log")
            self.results.append({"check": "Linux " + name + " with dedicated PostgreSQL", "result": "passed" if result.returncode == 0 else "failed"})
            print(("PASS " if result.returncode == 0 else "FAIL ") + "Linux " + name, flush=True)
            failed |= result.returncode != 0
        return not failed

    def lan_game(self):
        def configure(ports, discovery=True):
            game = next(g for g in self.rpc("alice", "games") if g["id"] == "minecraft-java")
            update = {"username": "test-admin", "password": self.admin_password,
                      "path": "games/" + game["id"], "method": "PUT", "body": {
                          "name": game["name"], "revision": game["revision"], "enabled": True,
                          "ports": ports, "network": {"version": 1, "broadcast": discovery,
                                                       "multicast": discovery, "ethernet_types": [0, 33079]}}}
            result = self.execute("control", "python3", "/opt/test/admin.py", input=json.dumps(update))
            return json.loads(result.stdout)["revision"]

        configure([{"protocol": protocol, "port": 1, "port_end": 65535} for protocol in ("tcp", "udp")])
        created = self.rpc("alice", "create", body={"name": "Ethernet LAN regression", "game": "minecraft-java"})
        self.rpc("bob", "join", body={"code": self.invite(created["invitation"])})

        def ready(service):
            state = self.connected(service)
            return state if state and state.get("lan", {}).get("ready") else None

        peers = {service: self.eventually(service + " TAP ready", lambda service=service: ready(service))
                 for service in ("alice", "bob")}
        for service in peers:
            link = json.loads(self.execute(service, "ip", "-j", "link", "show", "nodelane0-lan").stdout)[0]
            self.require(link["mtu"] == 1500, "game TAP MTU is not 1500")
            # Only isolated test containers are changed; no host routes or firewall.
            self.execute(service, "ip", "link", "set", "eth0", "mtu", "1280")
            self.fixture(service, "listen-game-probe-port")
            self.fixture(service, "listen-discovery", ip=peers[service]["ip"])
        for interface in ("eth0", "eth1"):
            self.execute("relay", "ip", "link", "set", interface, "mtu", "1280")
        probe_evidence = {}
        for service, other in (("alice", "bob"), ("bob", "alice")):
            own, peer = peers[service], peers[other]
            route = json.loads(self.execute(service, "ip", "-j", "route", "get", peer["ip"]).stdout)[0]
            self.require(route["dev"] == "nodelane0-lan", "LAN traffic bypasses TAP")
            self.eventually("LAN TCP", lambda: self.echo(service, peer["ip"]))
            for address, size in ((peer["ip"], 1472), (peer["ip"], 32000),
                                  (peer["lan"]["ipv6"], 1452), (peer["lan"]["ipv6"], 32000)):
                self.eventually("LAN UDP payload " + str(size), lambda: self.fixture(
                    service, "echo", ip=address, protocol="udp", port=26001, size=size)["ok"])
            self.require(self.echo(service, peer["ip"], "udp", 4243), "game UDP 4243 conflicts with diagnostics")
            prefix = self.execute(service, "ip", "-j", "-4", "addr", "show", "nodelane0-lan")
            bits = json.loads(prefix.stdout)[0]["addr_info"][0]["prefixlen"]
            directed = str(ipaddress.ip_network(own["ip"] + "/" + str(bits), strict=False).broadcast_address)
            for destination in ("255.255.255.255", directed, "239.10.20.30"):
                self.eventually("LAN discovery " + destination, lambda: self.fixture(
                    service, "discover", ip=own["ip"], peer=peer["ip"], destination=destination)["ok"])
            self.eventually("LAN relay", lambda: any(p["device_id"] == peer["device_id"] and p["mode"] == "relay"
                                                     for p in self.status(service)["peers"]))
            samples = []
            for _ in range(3):
                try:
                    rtt = self.rpc(service, "ping", target=peer["device_id"])["rtt_ms"]
                    self.require(rtt > 0, "LAN probe returned an invalid RTT")
                    samples.append({"result": "reply", "rtt_ms": rtt})
                except RuntimeError as exc:
                    if "overlay probe timed out" not in str(exc):
                        raise
                    samples.append({"result": "timeout"})
                time.sleep(1)
            self.require(any(s["result"] == "reply" for s in samples), "all three LAN probes timed out")
            probe_evidence[service] = samples
        self.passed("real TAP MTU 1500 over MTU 1280 Nebula relay: IPv4/IPv6, 32KB UDP, broadcast/multicast, game UDP 4243 and probes", probes=probe_evidence)
        revision = configure([{"protocol": "udp", "port": 26002}], discovery=False)
        # This port was already allowed by the full-range policy. Its success
        # alone cannot prove that either client has applied the narrower policy.
        for service in peers:
            def applied(service=service):
                state = ready(service)
                return state and state["game"]["revision"] == revision
            self.eventually(service + " narrowed LAN policy applied", applied)
        self.eventually("LAN narrowed UDP", lambda: self.echo("alice", peers["bob"]["ip"], "udp", 26002))
        self.require(not self.echo("alice", peers["bob"]["ip"], "udp", 26001), "LAN removed port survives")
        self.require(not self.fixture("alice", "discover", ip=peers["alice"]["ip"], peer=peers["bob"]["ip"],
                                     destination="255.255.255.255")["ok"], "disabled broadcast survives")
        self.rpc("alice", "kick", body={"device_id": peers["bob"]["device_id"]})
        self.stopped("bob")
        self.require(not self.echo("alice", peers["bob"]["ip"], "udp", 26002), "revoked Ethernet traffic survives")
        self.rpc("alice", "close")
        self.stopped("alice")
        self.passed("LAN live policy narrowing, discovery disablement and member revocation remove real traffic and TAP")

    def finish(self):
        print("Removing this run's containers, private volumes and networks...", flush=True)
        result = self.command("--profile", "verify", "down", "--volumes", "--remove-orphans",
                              timeout=120, check=False, log="cleanup.log")
        self.results.append({"check": "cleanup", "result": "passed" if result.returncode == 0 else "failed"})
        images_ok = True
        if result.returncode == 0:
            candidates = {"nodelane-test:" + self.name, "nodelane-test-source:" + self.name}
            listed = subprocess.run(["docker", "image", "ls", "--format", "{{.Repository}}:{{.Tag}}"],
                                    capture_output=True, text=True, timeout=30)
            images_ok = listed.returncode == 0
            images = sorted(candidates & set(listed.stdout.splitlines()))
            if images:
                removed = subprocess.run(["docker", "image", "rm", *images], capture_output=True, text=True, timeout=120)
                (self.out / "cleanup-images.log").write_text(removed.stdout + removed.stderr, encoding="utf-8")
                images_ok = images_ok and removed.returncode == 0
            self.results.append({"check": "per-run test images removed", "result": "passed" if images_ok else "failed"})
        (self.out / "results.json").write_text(json.dumps({"project": self.name, "checks": self.results},
                                                        ensure_ascii=False, indent=2), encoding="utf-8")
        print("Results: " + str(self.out), flush=True)
        return result.returncode == 0 and images_ok


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--verify", action="store_true", help="also run Linux vet/test/race with the dedicated PostgreSQL database")
    args = parser.parse_args()
    run = TestRun()
    ok = False
    try:
        run.setup()
        run.isolation()
        run.business()
        run.configured_game()
        run.lan_game()
        ok = run.verify() if args.verify else True
    except (RuntimeError, ValueError, subprocess.SubprocessError, OSError) as exc:
        message = run.redact(str(exc))
        run.results.append({"check": "test run", "result": "failed", "error": message})
        print("FAIL " + message, flush=True)
        run.command("logs", "--no-color", "--tail", "80", "control", log="control-error.log", check=False)
        for service in ("alice", "bob"):
            try:
                (run.out / (service + "-status.json")).write_text(json.dumps(run.status(service), indent=2), encoding="utf-8")
            except (RuntimeError, ValueError, OSError):
                pass
    finally:
        ok = run.finish() and ok
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
