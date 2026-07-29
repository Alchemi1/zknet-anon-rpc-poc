#!/bin/bash
# Start all ZKNetwork mixnet services + dashboard
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
DASHBOARD_DIR="$PROJECT_DIR/dashboard"
WAIT_PKI="${1:-yes}"  # pass --no-wait to skip waiting for PKI

cleanup() {
  echo "Shutting down dashboard..."
  kill "$DASHBOARD_PID" 2>/dev/null || true
  wait "$DASHBOARD_PID" 2>/dev/null || true
}
trap cleanup EXIT

cd "$PROJECT_DIR"

echo "=== 1/6 Checking Docker..."
if ! docker info >/dev/null 2>&1; then
  echo "ERROR: Docker is not running"
  exit 1
fi

echo "=== 2/6 Stopping host walletshield (if any)..."
if systemctl --user status walletshield.service 2>/dev/null | grep -q "active"; then
  systemctl --user stop walletshield.service 2>/dev/null || true
  echo "Stopped systemd walletshield.service"
fi
# Also kill any leftover host walletshield process
pkill -f "com.zkn-client.app.*walletshield" 2>/dev/null || true
sleep 1

echo "=== 3/6 Starting mixnet containers..."
if docker ps --format "{{.Names}}" 2>/dev/null | grep -q "mix-dirauth-1"; then
  echo "Containers already running, skipping compose up"
else
  docker compose up -d
fi

echo "=== 4/6 Waiting for containers..."
for container in mix-dirauth-1 mix-dirauth-2 mix-1 mix-2 mix-3 mix-gateway mix-servicenode mix-client; do
  for i in $(seq 1 30); do
    if docker ps --format "{{.Names}}" 2>/dev/null | grep -q "$container"; then
      break
    fi
    sleep 1
  done
done
echo "All containers up"

if [ "$WAIT_PKI" != "--no-wait" ]; then
  echo "=== 4b/6 Waiting for PKI consensus..."
  timeout 180 bash -c '
    while true; do
      if docker logs mix-client 2>&1 | grep -q "PKI doc available"; then
        echo "PKI consensus reached"
        break
      fi
      sleep 3
    done
  ' && echo "PKI OK" || echo "WARNING: PKI wait timed out"
fi

process_running() {
  docker exec mix-client sh -c "pidof '$1' >/dev/null 2>&1" 2>/dev/null
}

echo "=== 5/6 Starting services inside mix-client..."
if process_running http-proxy-client; then
  echo "http-proxy-client already running"
else
  docker exec -d mix-client \
    /usr/local/bin/http-proxy-client \
    -cfg /var/lib/katzenpost/client/thinclient.toml \
    -port 9205 -ep http_proxy -log_level DEBUG
  echo "http-proxy-client started on :9205"
fi

if process_running walletshield-kps; then
  echo "walletshield-kps already running"
else
  docker exec -d mix-client \
    /usr/local/bin/walletshield-kps \
    -config /var/lib/katzenpost/client/thinclient.toml \
    -listen 127.0.0.1:9200 \
    -kps_listen 0.0.0.0:9201 \
    -log_level INFO
  echo "walletshield-kps started on :9200 (KPS :9201)"
fi

if process_running kps-monitor; then
  echo "kps-monitor already running"
else
  docker exec -d mix-client \
    /usr/local/bin/kps-monitor \
    -boot http://127.0.0.1:9200 \
    -http :9206 \
    -interval 30s
  echo "kps-monitor started on :9206"
fi

echo "=== 6/6 Starting dashboard..."
# Kill any existing dashboard on port 3517
fuser -k 3517/tcp 2>/dev/null || true
sleep 1
cd "$DASHBOARD_DIR"
if [ ! -d dist ] || [ ! -f dist/index.html ]; then
  echo "Building frontend..."
  npm run build 2>/dev/null || echo "WARNING: npm build failed, using raw index.html"
fi
python3 server.py &
DASHBOARD_PID=$!
echo "Dashboard started on http://127.0.0.1:3517 (PID $DASHBOARD_PID)"

echo ""
echo "=== All services running ==="
echo "  HTTP proxy:   http://127.0.0.1:9205/"
echo "  WalletShield: http://127.0.0.1:9200/"
echo "  KPS:          127.0.0.1:9201"
echo "  Dashboard:    http://127.0.0.1:3517"
echo ""
echo "Quick test: curl -X POST http://127.0.0.1:9205/ -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_blockNumber\",\"params\":[],\"id\":1}'"
echo ""
echo "Press Ctrl+C to stop the dashboard (containers keep running)"
wait "$DASHBOARD_PID"
