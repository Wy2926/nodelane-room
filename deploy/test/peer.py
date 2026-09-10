"""Container-local test fixture; game traffic uses real TCP/UDP sockets."""

import http.client
import http.server
import json
import os
import re
import socket
import socketserver
import struct
import sys
import threading
import time


class TCP(socketserver.BaseRequestHandler):
    def handle(self):
        try:
            while data := self.request.recv(65536):
                self.request.sendall(data)
        except OSError:
            pass


class UDP(socketserver.BaseRequestHandler):
    def handle(self):
        data, sock = self.request
        sock.sendto(data, self.client_address)


class TCPServer(socketserver.ThreadingTCPServer):
    allow_reuse_address = True
    daemon_threads = True


class Fixture:
    def __init__(self):
        self.held = None
        self.advertising = False
        self.announcements = []
        self.lock = threading.Lock()
        self.multicast = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        self.multicast.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self.multicast.bind(("", 4445))
        self.multicast.setsockopt(socket.IPPROTO_IP, socket.IP_ADD_MEMBERSHIP,
                                  socket.inet_aton("224.0.2.60") + socket.inet_aton("0.0.0.0"))
        # Linux IP_RECVTTL is 12; Python does not expose it on all supported builds.
        self.multicast.setsockopt(socket.IPPROTO_IP, 12, 1)
        threading.Thread(target=self.collect, daemon=True).start()
        threading.Thread(target=self.advertise, daemon=True).start()

    def collect(self):
        while True:
            data, ancillary, _, source = self.multicast.recvmsg(2048, 128)
            match = re.fullmatch(rb"\[MOTD\](.*?)\[/MOTD\]\[AD\](\d+)\[/AD\]", data)
            if match:
                ttl = next((struct.unpack("i", value)[0] for level, kind, value in ancillary
                            if level == socket.IPPROTO_IP and kind == socket.IP_TTL), None)
                with self.lock:
                    self.announcements.append({"motd": match[1].decode(), "port": int(match[2]),
                                               "source": source[0], "ttl": ttl, "time": time.monotonic()})
                    self.announcements = self.announcements[-64:]

    def advertise(self):
        sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        sock.setsockopt(socket.IPPROTO_IP, socket.IP_MULTICAST_TTL, 0)
        sock.setsockopt(socket.IPPROTO_IP, socket.IP_MULTICAST_LOOP, 1)
        while True:
            if self.advertising:
                sock.sendto(b"[MOTD]Docker simulated world[/MOTD][AD]25565[/AD]", ("224.0.2.60", 4445))
            time.sleep(1)

    def action(self, req):
        action = req["action"]
        if action in ("echo", "hold"):
            sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM if req.get("protocol") == "udp" else socket.SOCK_STREAM)
            sock.settimeout(req.get("timeout", 2))
            try:
                sock.connect((req["ip"], req["port"]))
                payload = b"nodelane-docker-real-payload"
                sock.sendall(payload)
                received = b""
                while len(received) < len(payload):
                    chunk = sock.recv(65536)
                    if not chunk:
                        break
                    received += chunk
                ok = received == payload
                if action == "hold" and ok:
                    if self.held:
                        self.held.close()
                    self.held = sock
                    sock = None
                return {"ok": ok}
            except OSError as exc:
                return {"ok": False, "reason": type(exc).__name__}
            finally:
                if sock:
                    sock.close()
        if action == "held":
            if not self.held:
                return {"state": "absent"}
            try:
                self.held.settimeout(1)
                self.held.sendall(b"still-connected")
                data = self.held.recv(65536)
                return {"state": "open" if data else "closed"}
            except TimeoutError:
                return {"state": "timeout"}
            except OSError:
                return {"state": "closed"}
        if action == "advertise":
            self.advertising = req["enabled"]
            return {"ok": True}
        if action == "announcements":
            with self.lock:
                return [item for item in self.announcements
                        if time.monotonic() - item["time"] < 5]
        raise ValueError("unknown fixture action")


def serve():
    for port in (26001, 26002, 25565):
        server = TCPServer(("0.0.0.0", port), TCP)
        threading.Thread(target=server.serve_forever, daemon=True).start()
    for port in (26001, 26002):
        server = socketserver.ThreadingUDPServer(("0.0.0.0", port), UDP)
        threading.Thread(target=server.serve_forever, daemon=True).start()
    fixture = Fixture()

    class Handler(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def do_GET(self):
            self.send_response(200 if os.path.exists("/state/client/agent.sock") else 503)
            self.end_headers()

        def do_POST(self):
            req = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            try:
                data = fixture.action(req)
                self.send_response(200)
            except (OSError, ValueError, KeyError):
                data = {"error": "fixture request failed"}
                self.send_response(400)
            self.end_headers()
            self.wfile.write(json.dumps(data).encode())

    http.server.HTTPServer(("127.0.0.1", 9080), Handler).serve_forever()


def request(mode):
    data = sys.stdin.buffer.read()
    conn = http.client.HTTPConnection("127.0.0.1", 9080, timeout=45)
    if mode == "rpc":
        conn.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        conn.sock.settimeout(45)
        conn.sock.connect("/state/client/agent.sock")
    conn.request("POST", "/rpc" if mode == "rpc" else "/", data, {"Content-Type": "application/json"})
    response = conn.getresponse()
    print(json.dumps({"status": response.status, "data": json.loads(response.read())}))
    conn.close()


if __name__ == "__main__":
    if sys.argv[1] == "serve":
        serve()
    else:
        request(sys.argv[1])
