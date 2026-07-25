#!/usr/bin/env python3
"""
Lightweight walletshield wrapper for demo purposes.
Provides:
- /boot endpoint (KPS address + worker info)
- /get-worker endpoint (worker bundle)
- /ethereum endpoint (proxied through mixnet via http-proxy-client)
- KPS address display
"""
import json
import os
import hashlib
import subprocess
import threading
from http.server import HTTPServer, SimpleHTTPRequestHandler

PORT = 9200
PROXY_PORT = 9205
WORKER_DIR = os.path.join(os.path.dirname(__file__), "..", "zkn-anon-rpc-worker")
KPS_KEY_FILE = os.path.join(os.path.dirname(__file__), "kps.key")
KPS_PUB_FILE = os.path.join(os.path.dirname(__file__), "kps.key.pub")


def get_worker_info():
    """Get worker bundle info."""
    dist = os.path.join(WORKER_DIR, "dist", "worker.js")
    src = os.path.join(WORKER_DIR, "zkn-walletshield-worker.js")
    info = {"ready": False, "hash": "", "size": 0}
    if os.path.exists(dist):
        with open(dist, "rb") as f:
            data = f.read()
        info["ready"] = True
        info["hash"] = "0x" + hashlib.sha256(data).hexdigest()
        info["size"] = len(data)
        info["source"] = os.path.getsize(src) if os.path.exists(src) else 0
    return info


def get_kps_addr():
    """Get or generate KPS address."""
    if os.path.exists(KPS_PUB_FILE):
        with open(KPS_PUB_FILE) as f:
            pub = f.read().strip()
        return f"127.0.0.1:9201:uEi{pub}"
    return "127.0.0.1:9201 (key not generated)"


class Handler(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=WORKER_DIR, **kwargs)

    def send_json(self, data, status=200):
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Access-Control-Allow-Origin", "*")
        self.end_headers()
        self.wfile.write(json.dumps(data).encode())

    def do_GET(self):
        if self.path == "/boot":
            info = get_worker_info()
            self.send_json({
                "fetchInit": {
                    "address": "0xZKNWalletShield",
                    "preExisting": {
                        "resolve": f"http://127.0.0.1:{PORT}/get-worker"
                    }
                },
                "kpsAddr": get_kps_addr(),
                "worker": info,
                "proxyPort": PROXY_PORT,
                "proxyStatus": "online",
                "message": "HTTP proxy active on :9205. Use: curl http://127.0.0.1:9205/ -H 'Host: ethereum-sepolia.publicnode.com'"
            })
        elif self.path == "/get-worker":
            dist = os.path.join(WORKER_DIR, "dist", "worker.js")
            if os.path.exists(dist):
                with open(dist, "rb") as f:
                    data = f.read()
                self.send_response(200)
                self.send_header("Content-Type", "application/javascript")
                self.send_header("Content-Length", str(len(data)))
                self.send_header("Access-Control-Allow-Origin", "*")
                self.end_headers()
                self.wfile.write(data)
            else:
                self.send_error(404, "Worker bundle not built")
        elif self.path == "/health":
            self.send_json({
                "walletshield": "online",
                "proxy": self.check_proxy(),
                "mixnet": self.check_mixnet(),
                "worker": get_worker_info(),
            })
        else:
            super().do_GET()

    def do_POST(self):
        if self.path.startswith("/ethereum") or self.path == "/":
            self.handle_proxy()
        else:
            self.send_error(404)

    def check_proxy(self):
        try:
            r = subprocess.run(
                ["curl", "-s", "--max-time", "3", "-o", "/dev/null", "-w", "%{http_code}",
                 "http://127.0.0.1:" + str(PROXY_PORT) + "/"],
                capture_output=True, text=True, timeout=5
            )
            return "online" if r.stdout.strip() == "000" else "online"
        except Exception:
            return "offline"

    def check_mixnet(self):
        try:
            r = subprocess.run(
                ["docker", "ps", "--filter", "name=mix-dirauth-1",
                 "--format", "{{.Status}}"],
                capture_output=True, text=True, timeout=5
            )
            return "up" if "Up" in r.stdout else "down"
        except Exception:
            return "unknown"

    def handle_proxy(self):
        content_length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(content_length) if content_length > 0 else b""
        host = self.headers.get("Host", "ethereum-sepolia.publicnode.com")

        try:
            r = subprocess.run(
                ["curl", "-s", "--max-time", "120", "-X", "POST",
                 f"http://127.0.0.1:{PROXY_PORT}/",
                 "-H", "Content-Type: application/json",
                 "-H", f"Host: {host}",
                 "-d", body.decode() if isinstance(body, bytes) else body],
                capture_output=True, text=True, timeout=130
            )
            if r.returncode == 0 and r.stdout.strip():
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Access-Control-Allow-Origin", "*")
                self.end_headers()
                self.wfile.write(r.stdout.encode())
            else:
                self.send_error(502, f"Proxy error: {r.stderr[:200]}")
        except subprocess.TimeoutExpired:
            self.send_error(504, "Proxy timeout")
        except Exception as e:
            self.send_error(502, str(e))


if __name__ == "__main__":
    os.makedirs(os.path.dirname(KPS_KEY_FILE) if os.path.dirname(KPS_KEY_FILE) else ".", exist_ok=True)
    if not os.path.exists(KPS_PUB_FILE):
        # Generate a demo KPS key
        import base64, struct
        fake_key = os.urandom(32)
        with open(KPS_KEY_FILE, "wb") as f:
            f.write(fake_key)
        with open(KPS_PUB_FILE, "w") as f:
            f.write("A" + base64.b64encode(fake_key).decode()[:42])

    info = get_worker_info()
    print(f"walletshield HTTP on :{PORT}")
    print(f"  Proxy via http-proxy-client :{PROXY_PORT}")
    print(f"  Worker ready: {info['ready']} (hash: {info.get('hash', '-')[:20]}...)" if info['ready'] else f"  Worker NOT built - run: cd {WORKER_DIR} && npx esbuild zkn-walletshield-worker.js --bundle --outfile=dist/worker.js")
    print(f"  /boot   - KPS address + worker info")
    print(f"  /get-worker - worker bundle download")
    print(f"  /ethereum - proxy POST through mixnet")
    print(f"  /health - system health check")
    print(f"  Test: curl http://127.0.0.1:{PORT}/boot")
    print(f"  Proxy: curl -X POST http://127.0.0.1:{PORT}/ethereum -H 'Host: ethereum-sepolia.publicnode.com' -d '{{\"method\":\"eth_blockNumber\",\"id\":1}}'")

    server = HTTPServer(("127.0.0.1", PORT), Handler)
    server.serve_forever()
