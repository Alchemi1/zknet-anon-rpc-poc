#!/bin/bash
# ZKNetwork / Anon-RPC — one-shot bootstrap + start
#   First run:  git clone && cd zknet-anon-rpc-poc && ./start.sh
#   Subsequent: ./start.sh  (skips already-built artifacts)
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJECT_DIR"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'
ok()   { echo -e "  ${GREEN}[OK]${NC} $1"; }
warn() { echo -e "  ${YELLOW}[WARN]${NC} $1"; }
fail() { echo -e "  ${RED}[FAIL]${NC} $1"; exit 1; }

echo "============================================"
echo " ZKNetwork Anon-RPC — Bootstrap + Start"
echo " $(date)"
echo "============================================"

# ── 0. Prerequisites ──────────────────────────────────────────────

echo ""
echo "── Checking prerequisites ──"

if ! docker info >/dev/null 2>&1; then
  fail "Docker is not running. Start Docker and retry."
fi
ok "Docker running"

if [ ! -f katzenpost/go.mod ]; then
  echo "  Cloning katzenpost submodule..."
  git submodule update --init --recursive 2>&1 || fail "git submodule failed"
  ok "katzenpost submodule cloned"
else
  ok "katzenpost submodule present"
fi

# ── 1. Build mixnet Docker image ──────────────────────────────────

echo ""
echo "── Building mixnet image ──"
MIXNET_IMAGE="zeros/mixnet-node:amd64"
if docker image inspect "$MIXNET_IMAGE" >/dev/null 2>&1; then
  ok "mixnet image already built: $MIXNET_IMAGE"
else
  echo "  Building Docker image (first time: ~15 min)..."
  docker build -t "$MIXNET_IMAGE" -f Dockerfile.mixnet . 2>&1 || fail "Docker build failed"
  ok "mixnet image built: $MIXNET_IMAGE"
fi

# ── 2. Build walletshield binary ──────────────────────────────────

echo ""
echo "── Building walletshield ──"
WS_BIN="config/mixnet/client/walletshield-kps"
if [ -x "$WS_BIN" ]; then
  ok "walletshield-kps binary present"
else
  echo "  Building walletshield (Docker, first time: ~3 min)..."
  WS_IMG="walletshield-build:local"
  docker build -t "$WS_IMG" -f Dockerfile.walletshield.local . 2>&1 || fail "walletshield build failed"
  CID=$(docker create "$WS_IMG" 2>/dev/null)
  docker cp "$CID:/usr/local/bin/walletshield" "$WS_BIN" 2>/dev/null || fail "copy walletshield binary failed"
  docker rm "$CID" >/dev/null 2>&1
  ok "walletshield-kps built -> $WS_BIN"
fi

# ── 3. Build Go tools (kps-monitor, kps-client, kps-sendtx) ──────

echo ""
echo "── Building Go tools ──"

build_go_binary() {
  local dir="$1" binary="$2"
  if [ -x "$dir/$binary" ]; then
    ok "$binary present"
    return
  fi
  echo "  Building $binary..."
  docker run --rm -v "$PROJECT_DIR/$dir:/src" -w /src golang:latest \
    go build -trimpath -o "$binary" . 2>&1 || warn "$binary build skipped (optional)"
  if [ -x "$dir/$binary" ]; then
    ok "$binary built"
  fi
}

build_go_binary kps-monitor  kps-monitor
build_go_binary kps-client   kps-client
build_go_binary kps-sendtx   kps-sendtx

# ── 4. Start mixnet containers ────────────────────────────────────

echo ""
echo "── Starting mixnet containers ──"

if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "mix-dirauth-1"; then
  ok "mixnet containers already running"
else
  docker compose up -d 2>&1 || fail "docker compose up failed"
  ok "containers starting..."
fi

# ── 5. Wait for containers ────────────────────────────────────────

echo ""
echo "── Waiting for containers ──"
for container in mix-dirauth-1 mix-dirauth-2 mix-dirauth-3 \
                 mix-1 mix-2 mix-3 mix-gateway mix-servicenode mix-client; do
  for i in $(seq 1 60); do
    if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "$container"; then
      break
    fi
    sleep 1
  done
done
ok "all 9 containers up"

# ── 5b. Ensure container DNS ───────────────────────────────────────
# After host reboot / docker recreation, some containers can end up with an
# empty /etc/resolv.conf (no nameservers), which breaks the servicenode's
# http_proxy upstream lookups (DNS resolution of the RPC endpoint). Fix by
# writing the host's nameservers into any container missing them.

echo ""
echo "── Ensuring container DNS ──"
HOST_NS=$(grep -m3 '^nameserver' /etc/resolv.conf 2>/dev/null | awk '{print $2}' | head -3)
if [ -z "$HOST_NS" ]; then
  warn "host has no nameservers in /etc/resolv.conf — skipping DNS fix"
else
  NS_ENTRIES=$(printf 'nameserver %s\n' $HOST_NS)
  DNS_FIXED=0
  for container in mix-dirauth-1 mix-dirauth-2 mix-dirauth-3 \
                   mix-1 mix-2 mix-3 mix-gateway mix-servicenode mix-client; do
    if ! docker exec "$container" sh -c "grep -q '^nameserver' /etc/resolv.conf" 2>/dev/null; then
      docker exec "$container" sh -c "printf '%b\n' '$NS_ENTRIES' > /etc/resolv.conf"
      ok "$container: resolv.conf fixed ($(echo $HOST_NS | tr ' ' ', '))"
      DNS_FIXED=1
    fi
  done
  if [ "$DNS_FIXED" -eq 0 ]; then
    ok "all containers have nameservers"
  fi
fi

# ── 6. Wait for PKI consensus ─────────────────────────────────────

echo ""
echo "── Waiting for PKI consensus ──"

PKI_OK="no"
for i in $(seq 1 120); do
  if docker logs mix-client 2>&1 | grep -q "PKI doc available"; then
    PKI_OK="yes"
    break
  fi
  sleep 3
done

if [ "$PKI_OK" = "yes" ]; then
  ok "PKI consensus reached"
else
  # Check if consensus exists despite log message missing
  if docker exec mix-client sh -c "/usr/local/bin/fetch -f /var/lib/katzenpost/client/thinclient.toml" 2>/dev/null; then
    ok "PKI doc available (manual fetch)"
  else
    warn "PKI still converging — services may take longer"
  fi
fi

# ── 7. Start services ─────────────────────────────────────────────

echo ""
echo "── Starting services ──"

# http-proxy-client (inside mix-client)
if docker exec mix-client sh -c "pidof http-proxy-client >/dev/null 2>&1" 2>/dev/null; then
  ok "http-proxy-client already running"
else
  docker exec -d mix-client \
    /usr/local/bin/http-proxy-client \
    -cfg /var/lib/katzenpost/client/thinclient.toml \
    -port 9205 -ep http_proxy -log_level INFO
  ok "http-proxy-client started on :9205"
fi

# walletshield-kps (inside mix-client; binary baked into image at /usr/local/bin)
# NOTE: must run with cwd=/var/lib/katzenpost/client so the relative kps.key
# path resolves and the KPS listener (:9201/UDP) binds correctly.
if docker exec mix-client sh -c "pidof walletshield-kps >/dev/null 2>&1" 2>/dev/null; then
  ok "walletshield-kps already running"
else
  docker exec -d -w /var/lib/katzenpost/client mix-client \
    /usr/local/bin/walletshield-kps \
    -config /var/lib/katzenpost/client/thinclient.toml \
    -listen 127.0.0.1:9200 \
    -kps_listen 0.0.0.0:9201 \
    -log_level INFO
  ok "walletshield-kps started :9200 (KPS :9201)"
fi

# kps-monitor (inside mix-client; binary baked into image at /usr/local/bin)
if docker exec mix-client sh -c "pidof kps-monitor >/dev/null 2>&1" 2>/dev/null; then
  ok "kps-monitor already running"
else
  docker exec -d mix-client \
    /usr/local/bin/kps-monitor \
    -boot http://127.0.0.1:9200 \
    -http :9206 \
    -interval 15s
  ok "kps-monitor started on :9206"
fi

# ── 8. Dashboard ──────────────────────────────────────────────────

echo ""
echo "── Starting dashboard ──"
DASHBOARD_DIR="$PROJECT_DIR/dashboard"

if [ ! -d "$DASHBOARD_DIR/node_modules" ]; then
  echo "  Installing npm dependencies..."
  (cd "$DASHBOARD_DIR" && npm install) 2>&1 || warn "npm install had warnings"
fi

if [ ! -f "$DASHBOARD_DIR/dist/index.html" ]; then
  echo "  Building frontend..."
  (cd "$DASHBOARD_DIR" && npx vite build) 2>&1 || warn "vite build failed"
fi

fuser -k 3517/tcp 2>/dev/null || true
sleep 1

cd "$DASHBOARD_DIR"
nohup python3 server.py > /tmp/dashboard.log 2>&1 &
DASHBOARD_PID=$!
ok "dashboard started http://127.0.0.1:3517 (PID $DASHBOARD_PID)"

# ── 9. Health check and summary ───────────────────────────────────

echo ""
echo "============================================"
echo " ZKNetwork Anon-RPC — Live"
echo " $(date)"
echo "============================================"
echo ""
echo "  HTTP proxy:   http://127.0.0.1:9205/"
echo "  WalletShield: http://127.0.0.1:9200/"
echo "  KPS:          127.0.0.1:9201"
echo "  KPS Monitor:  http://127.0.0.1:9206/stats"
echo "  Dashboard:    http://127.0.0.1:3517"
echo ""

sleep 5
echo "── Quick health check ──"
HC_OK=0
HC_FAIL=0

check() {
  local label="$1" cmd="$2"
  if eval "$cmd" >/dev/null 2>&1; then
    ok "$label" ; HC_OK=$((HC_OK+1))
  else
    warn "$label" ; HC_FAIL=$((HC_FAIL+1))
  fi
}

check "walletshield boot"     'curl -s --max-time 5 http://127.0.0.1:9200/boot | grep -q kpsAddr'
check "KPS monitor"           'curl -s --max-time 5 http://127.0.0.1:9206/stats | grep -q connected'
check "dashboard"             'curl -s --max-time 5 http://127.0.0.1:3517/api/containers | grep -q active'
check "HTTP proxy"            'timeout 40 curl -s --max-time 35 -X POST http://127.0.0.1:9205/ -H "Host: ethereum-sepolia.publicnode.com" -H "Content-Type: application/json" -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_blockNumber\",\"params\":[],\"id\":1}" | grep -q result'

echo ""
if [ $HC_FAIL -eq 0 ]; then
  echo -e "${GREEN}All $HC_OK checks passed.${NC}  Run ./demo.sh for the full test suite."
else
  echo -e "${YELLOW}$HC_OK passed, $HC_FAIL failed.${NC}  Check logs above, may need more time for consensus."
fi
echo ""
echo "To stop:  ./stop.sh"
