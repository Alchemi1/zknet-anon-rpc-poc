#!/usr/bin/env python3
"""Dashboard HTTP server — serves frontend + API endpoints."""
import http.server
import json
import subprocess
import re
import os
import time
import hashlib

PORT = 3517
BASE_DIR = os.path.dirname(os.path.abspath(__file__))
PROJECT_DIR = os.path.dirname(BASE_DIR)
DIST = os.path.join(BASE_DIR, "dist")
CONFIG_DIR = os.path.join(PROJECT_DIR, "config", "mixnet")

AUTH_NAMES = ["auth1", "auth2", "auth3"]
CONTAINER_NAMES = ["mix-dirauth-1", "mix-dirauth-2", "mix-dirauth-3",
                   "mix-1", "mix-2", "mix-3", "mix-gateway", "mix-servicenode", "mix-client"]


def find_auth_containers():
    """Dynamically discover running dirauth containers by name pattern."""
    try:
        r = subprocess.run(
            ["docker", "ps", "--format", "{{.Names}}"],
            capture_output=True, text=True, timeout=5
        )
        return [n.strip() for n in r.stdout.splitlines() if "dirauth" in n]
    except Exception:
        return ["mix-dirauth-1", "mix-dirauth-2", "mix-dirauth-3"]


def find_container_strict(name):
    """Find a container whose name ends with the given name."""
    try:
        r = subprocess.run(
            ["docker", "ps", "--format", "{{.Names}}"],
            capture_output=True, text=True, timeout=5
        )
        for n in r.stdout.splitlines():
            if n.strip().endswith(name):
                return n.strip()
    except Exception:
        pass
    return name


class DashboardHandler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=DIST, **kwargs)

    def do_GET(self):
        if self.path == "/api/containers":
            return self.api_containers()
        if self.path == "/api/epoch":
            return self.api_epoch()
        if self.path == "/api/anonrpc":
            return self.api_anonrpc()
        if self.path.startswith("/api/logs/"):
            container = self.path.split("/")[-1]
            return self.api_logs(container)
        if self.path == "/api/kps-stats":
            return self.api_kps_stats()
        if self.path == "/api/kps-rpc":
            return self.api_kps_rpc()
        if self.path == "/api/test":
            return self.api_test()
        return super().do_GET()

    def do_POST(self):
        if self.path == "/api/test":
            return self.api_test()
        if self.path == "/api/kps-rpc":
            return self.api_kps_rpc()
        return self.send_error(404)

    def send_json(self, data):
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Access-Control-Allow-Origin", "*")
        self.end_headers()
        self.wfile.write(json.dumps(data).encode())

    def docker_ps(self, name):
        try:
            r = subprocess.run(
                ["docker", "ps", "--filter", f"name={name}", "--format", "{{.Names}}\t{{.Status}}\t{{.Ports}}"],
                capture_output=True, text=True, timeout=5
            )
            parts = r.stdout.strip().split("\t")
            return {"name": parts[0], "status": parts[1], "ports": parts[2] if len(parts) > 2 else ""}
        except Exception:
            return {"name": name, "status": "down", "ports": ""}

    def anon_rpc_worker_info(self):
        worker_dir = os.path.join(PROJECT_DIR, "zkn-anon-rpc-worker")
        dist_path = os.path.join(worker_dir, "dist", "worker.js")
        src_path = os.path.join(worker_dir, "zkn-walletshield-worker.js")
        contract_path = os.path.join(worker_dir, "ZKNWalletShield.sol")
        info = {"worker_ready": False, "worker_hash": "", "contract_exists": False, "source_lines": 0}
        if os.path.exists(dist_path):
            with open(dist_path, "rb") as f:
                data = f.read()
            info["worker_ready"] = True
            info["worker_hash"] = "0x" + hashlib.sha256(data).hexdigest()
            info["worker_size"] = len(data)
        if os.path.exists(src_path):
            with open(src_path) as f:
                info["source_lines"] = len(f.readlines())
        if os.path.exists(contract_path):
            info["contract_exists"] = True
        return info

    def kps_status(self):
        try:
            r = subprocess.run(["curl", "-s", "--max-time", "3", "http://127.0.0.1:9200/boot"],
                               capture_output=True, text=True, timeout=5)
            if r.returncode == 0 and r.stdout.strip():
                try:
                    boot = json.loads(r.stdout)
                    return {"listening": True, "boot": boot, "port": 9200}
                except json.JSONDecodeError:
                    return {"listening": True, "boot": None, "port": 9200}
        except Exception:
            pass
        return {"listening": False, "boot": None, "port": 9201}

    def api_anonrpc(self):
        ws = self.kps_status()
        worker = self.anon_rpc_worker_info()
        kps_boot = ws.get("boot", {}) if ws.get("boot") else {}
        self.send_json({
            "kps": ws,
            "worker": worker,
            "http_proxy_port": 9205,
            "kps_port": 9201,
            "walletshield_port": 9200,
            "kps_addr": kps_boot.get("kpsAddr", "N/A"),
            "worker_hash": kps_boot.get("worker", {}).get("hash", worker.get("worker_hash", "")),
        })

    def read_auth_log(self, container_name):
        """Read auth log from its container. container_name is like mix-dirauth-1 or hash_mix-dirauth-3."""
        m = re.search(r'dirauth[_-](\d+)', container_name)
        auth_dir = f"auth{m.group(1)}" if m else container_name
        try:
            r = subprocess.run(
                ["docker", "exec", container_name, "tail", "-n", "500", f"/var/lib/katzenpost/{auth_dir}/katzenpost.log"],
                capture_output=True, text=True, timeout=10
            )
            if r.returncode == 0:
                return r.stdout.splitlines()
        except Exception:
            pass
        return []

    def api_containers(self):
        containers = [self.docker_ps(n) for n in CONTAINER_NAMES]
        # Discover any additional auth containers with hash prefixes
        discovered = find_auth_containers()
        seen = {c["name"] for c in containers}
        for dc in discovered:
            if dc not in seen:
                containers.append(self.docker_ps(dc))
                seen.add(dc)
        active = sum(1 for c in containers if "Up" in c["status"])
        total = len(containers)
        self.send_json({"containers": containers, "active": active, "total": total})

    def api_epoch(self):
        last_consensus = ""
        current_epoch = ""
        consensus_times = []
        auth_containers = find_auth_containers()

        for container in auth_containers:
            # Derive auth dir name: mix-dirauth-3 → auth3
            m = re.search(r'dirauth[_-](\d+)', container)
            auth_name = f"auth{m.group(1)}" if m else container
            lines = self.read_auth_log(container)
            for line in lines:
                if "SUCCESS! Achieved threshold consensus" in line and "Epoch:" in line:
                    m = re.search(r"Epoch:\s*(\d+)", line)
                    if m:
                        consensus_times.append(m.group(1))

        consensus_count = len(set(consensus_times))
        last_consensus = consensus_times[-1] if consensus_times else ""

        # Get current epoch from client logs
        try:
            r = subprocess.run(
                ["docker", "exec", "mix-client", "sh", "-c",
                 "tail -200 /var/lib/katzenpost/client/thinclient.log /var/lib/katzenpost/client/client.log 2>/dev/null"],
                capture_output=True, text=True, timeout=10
            )
            for line in r.stdout.splitlines():
                m = re.search(r"(?:epoch|Epoch)[:\s]*(\d+)", line)
                if m:
                    current_epoch = m.group(1)
        except Exception:
            pass

        # Fallback: also try the walletshield's PKI doc output
        if not current_epoch:
            try:
                r = subprocess.run(
                    ["docker", "exec", "mix-client", "sh", "-c",
                     "grep 'PKI doc epoch' /tmp/wl4.log /tmp/walletshield.log 2>/dev/null | tail -1"],
                    capture_output=True, text=True, timeout=5
                )
                m = re.search(r"epoch=(\d+)", r.stdout)
                if m:
                    current_epoch = m.group(1)
            except Exception:
                pass

        # Also try querying the walletshield boot endpoint for the live epoch
        try:
            r = subprocess.run(
                ["curl", "-s", "--max-time", "3", "http://127.0.0.1:9200/boot"],
                capture_output=True, text=True, timeout=5
            )
        except Exception:
            pass

        status = "consensus OK" if consensus_count >= 3 else f"partial ({consensus_count}/3)" if consensus_count > 0 else "no consensus"
        self.send_json({
            "current_epoch": current_epoch or last_consensus or "N/A",
            "consensus_status": status,
            "last_consensus": f"Epoch {last_consensus}" if last_consensus else "N/A",
        })

    def api_logs(self, container):
        try:
            r = subprocess.run(["docker", "logs", container, "--tail", "30"],
                               capture_output=True, text=True, timeout=5)
            lines = [{"time": (l[:15] if len(l) > 15 else ""), "text": l}
                     for l in r.stdout.splitlines()]
        except Exception as e:
            lines = [{"time": "", "text": f"error: {e}"}]
        self.send_json(lines)

    def api_kps_stats(self):
        try:
            r = subprocess.run(
                ["curl", "-s", "--max-time", "5", "http://127.0.0.1:9206/stats"],
                capture_output=True, text=True, timeout=6
            )
            if r.returncode == 0 and r.stdout.strip():
                data = json.loads(r.stdout)
                self.send_json(data)
                return
        except Exception:
            pass
        self.send_json({"connected": False, "error": "kps-monitor unreachable"})

    def api_kps_rpc(self):
        content_length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(content_length) if content_length > 0 else b"{}"
        try:
            r = subprocess.run(
                ["curl", "-s", "--max-time", "60", "-X", "POST", "http://127.0.0.1:9206/rpc",
                 "-H", "Content-Type: application/json",
                 "-d", body.decode()],
                capture_output=True, text=True, timeout=65
            )
            if r.returncode == 0 and r.stdout.strip():
                self.send_json(json.loads(r.stdout))
                return
        except subprocess.TimeoutExpired:
            self.send_json({"error": "KPS RPC timeout (60s)", "rtt_ms": 60000})
            return
        except Exception as e:
            self.send_json({"error": str(e)})
            return
        self.send_json({"error": "KPS RPC failed", "rtt_ms": 0})

    def api_test(self):
        import time, subprocess
        start = time.time()
        data = '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
        try:
            r = subprocess.run(
                ["curl", "-s", "--max-time", "120", "-X", "POST", "http://127.0.0.1:9205/",
                 "-H", "Content-Type: application/json",
                 "-H", "Host: ethereum-sepolia.publicnode.com",
                 "-d", data],
                capture_output=True, text=True, timeout=130
            )
            duration = int((time.time() - start) * 1000)
            stdout = r.stdout.strip()
            stderr = r.stderr.strip()
            success = '"result"' in stdout
            self.send_json({
                "success": success,
                "response": stdout[:2000] if stdout else "(empty)",
                "duration_ms": duration,
                "error": stderr if stderr else ""
            })
        except subprocess.TimeoutExpired:
            duration = int((time.time() - start) * 1000)
            self.send_json({"success": False, "response": "", "duration_ms": duration, "error": "timeout (120s)"})
        except Exception as e:
            duration = int((time.time() - start) * 1000)
            self.send_json({"success": False, "response": "", "duration_ms": duration, "error": str(e)})


if __name__ == "__main__":
    os.makedirs(DIST, exist_ok=True)
    if not os.listdir(DIST):
        import shutil
        src = os.path.join(os.path.dirname(__file__), "index.html")
        if os.path.exists(src):
            shutil.copy(src, os.path.join(DIST, "index.html"))

    print(f"Dashboard at http://127.0.0.1:{PORT}")
    server = http.server.ThreadingHTTPServer(("127.0.0.1", PORT), DashboardHandler)
    server.serve_forever()
