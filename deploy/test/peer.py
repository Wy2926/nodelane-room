"""Container-local test fixture; game traffic uses real TCP/UDP sockets."""

import http.client
import http.server
import json
import os
import socket
import socketserver
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


class UDP6Server(socketserver.ThreadingUDPServer):
    address_family = socket.AF_INET6
    max_packet_size = 65535

    def server_bind(self):
        self.socket.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_V6ONLY, 1)
        super().server_bind()


class Fixture:
    def __init__(self):
        self.held = None
        self.discovery = None

    def action(self, req):
        action = req["action"]
        if action == "listen-game-probe-port":
            server = socketserver.ThreadingUDPServer(("0.0.0.0", 4243), UDP)
            threading.Thread(target=server.serve_forever, daemon=True).start()
            return {"ok": True}
        if action == "listen-discovery":
            if self.discovery:
                self.discovery.close()
            sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            sock.bind(("", 27077))
            sock.setsockopt(socket.IPPROTO_IP, socket.IP_ADD_MEMBERSHIP,
                            socket.inet_aton("239.10.20.30") + socket.inet_aton(req["ip"]))
            self.discovery = sock

            def respond():
                try:
                    while True:
                        data, source = sock.recvfrom(65535)
                        sock.sendto(data, source)
                except OSError:
                    pass

            threading.Thread(target=respond, daemon=True).start()
            return {"ok": True}
        if action == "discover":
            with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
                sock.bind((req["ip"], 0))
                sock.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)
                sock.setsockopt(socket.IPPROTO_IP, socket.IP_MULTICAST_IF, socket.inet_aton(req["ip"]))
                sock.setsockopt(socket.IPPROTO_IP, socket.IP_MULTICAST_TTL, 1)
                sock.settimeout(2)
                payload = os.urandom(128)
                sock.sendto(payload, (req["destination"], 27077))
                deadline = time.monotonic() + 2
                try:
                    while time.monotonic() < deadline:
                        data, source = sock.recvfrom(65535)
                        if data == payload and source[0] == req["peer"]:
                            return {"ok": True, "source": source[0]}
                except OSError:
                    pass
                return {"ok": False}
        if action in ("echo", "hold"):
            sock = socket.socket(socket.AF_INET6 if ":" in req["ip"] else socket.AF_INET, socket.SOCK_DGRAM if req.get("protocol") == "udp" else socket.SOCK_STREAM)
            sock.settimeout(req.get("timeout", 2))
            try:
                sock.connect((req["ip"], req["port"]))
                payload = os.urandom(min(req.get("size", 32), 60000))
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
        raise ValueError("unknown fixture action")


def serve():
    for port in (26001, 26002, 25565):
        server = TCPServer(("0.0.0.0", port), TCP)
        threading.Thread(target=server.serve_forever, daemon=True).start()
    for port in (26001, 26002):
        server = socketserver.ThreadingUDPServer(("0.0.0.0", port), UDP)
        server.max_packet_size = 65535
        threading.Thread(target=server.serve_forever, daemon=True).start()
        server = UDP6Server(("::", port), UDP)
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
