#!/bin/bash
# ZKNetwork / Anon-RPC Demo Script
# Tests all transport paths: HTTP proxy, KPS CLI, KPS monitor, Dashboard
set -uo pipefail

PASS=0
FAIL=0
SKIP=0

ok()   { PASS=$((PASS+1)); echo "  [PASS] $1"; }
fail() { FAIL=$((FAIL+1)); echo "  [FAIL] $1"; echo "         $2"; }
skip() { SKIP=$((SKIP+1)); echo "  [SKIP] $1"; }

try_curl() {
  local timeout=$1 url=$2; shift 2
  curl -s --max-time "$timeout" "$url" "$@" 2>/dev/null || echo ""
}

echo "============================================"
echo " ZKNetwork Anon-RPC Demo"
echo " $(date)"
echo "============================================"
echo ""

# 1. Walletshield boot - fast, no mixnet
echo "--- 1. Walletshield Boot ---"
BOOT=$(try_curl 5 http://127.0.0.1:9200/boot)
KPS_ADDR=$(echo "$BOOT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('kpsAddr',''))" 2>/dev/null)
if [ -n "$KPS_ADDR" ]; then
  ok "Boot endpoint: kpsAddr=$KPS_ADDR"
else
  fail "Boot endpoint" "$BOOT"
fi

# 2-6: mixnet-dependent - check walletshield is up via /boot (returns instantly)
WS_KPS=$(try_curl 3 http://127.0.0.1:9200/boot | python3 -c "import sys,json;print(json.load(sys.stdin).get('kpsAddr',''))" 2>/dev/null)
if [ -z "$WS_KPS" ]; then
  echo ""
  echo "  (walletshield unreachable, skipping mixnet tests)"
  skip "HTTP Proxy (walletshield)"
  skip "HTTP Proxy (direct)"
  skip "KPS Client"
  skip "KPS Monitor probes"
  skip "KPS RPC on-demand"
else
  # 2. HTTP proxy via walletshield (port 9200) - may block
  echo ""
  echo "--- 2. HTTP Proxy (walletshield :9200) ---"
  R=$(timeout 35 curl -s --max-time 30 -X POST http://127.0.0.1:9200/ethereum \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' 2>/dev/null || echo "")
  B=$(echo "$R" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('result',''))" 2>/dev/null || echo "")
  if [ -n "$B" ]; then
    ok "Walletshield HTTP proxy: $B"
  else
    fail "Walletshield HTTP proxy" "$(echo "$R" | head -c 200)"
  fi

  # 3. Direct HTTP proxy (port 9205)
  echo ""
  echo "--- 3. HTTP Proxy (direct :9205) ---"
  R=$(timeout 35 curl -s --max-time 30 -X POST http://127.0.0.1:9205/ \
    -H "Content-Type: application/json" \
    -H "Host: ethereum-sepolia.publicnode.com" \
    -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' 2>/dev/null || echo "")
  B=$(echo "$R" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('result',''))" 2>/dev/null || echo "")
  if [ -n "$B" ]; then
    ok "Direct HTTP proxy: $B"
  else
    fail "Direct HTTP proxy" "$(echo "$R" | head -c 200)"
  fi

  # 4. KPS Client
  echo ""
  echo "--- 4. KPS Client ---"
  if [ -x ./kps-client/kps-client ]; then
    KPS_CLIENT=$(timeout 120 ./kps-client/kps-client -boot http://127.0.0.1:9200 2>&1 || echo "TIMEOUT/ERROR")
    B=$(echo "$KPS_CLIENT" | grep "^Result:" | sed 's/Result: //')
    if [ -n "$B" ]; then
      ok "KPS client: $B"
    else
      fail "KPS client" "$(echo "$KPS_CLIENT" | tail -3)"
    fi
  else
    skip "KPS client (binary not found)"
  fi

  # 5. KPS Monitor /stats
  echo ""
  echo "--- 5. KPS Monitor ---"
  STATS=$(try_curl 5 http://127.0.0.1:9206/stats)
  C=$(echo "$STATS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('connected',''))" 2>/dev/null || echo "")
  if [ "$C" = "True" ]; then
    ok "KPS monitor: connected"
  else
    fail "KPS monitor" "$STATS"
  fi

  # 6. KPS RPC on-demand
  echo ""
  echo "--- 6. KPS RPC (on-demand) ---"
  R=$(timeout 65 curl -s --max-time 60 -X POST http://127.0.0.1:9206/rpc \
    -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' 2>/dev/null || echo '{"error":"timeout"}')
  B=$(echo "$R" | python3 -c "import sys,json; print(json.load(sys.stdin).get('result',''))" 2>/dev/null || echo "")
  RTT_MS=$(echo "$R" | python3 -c "import sys,json; print(json.load(sys.stdin).get('rtt_ms',''))" 2>/dev/null || echo "")
  if [ -n "$B" ]; then
    ok "KPS RPC: $B (${RTT_MS}ms)"
  else
    fail "KPS RPC" "$(echo "$R" | head -c 200)"
  fi
fi

# 7-8: dashboard - independent of mixnet
echo ""
echo "--- 7. Dashboard ---"
CONT=$(try_curl 5 http://127.0.0.1:3517/api/containers)
ACTIVE=$(echo "$CONT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('active',0))" 2>/dev/null || echo "0")
TOTAL=$(echo "$CONT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('total',0))" 2>/dev/null || echo "0")
if [ "$ACTIVE" -ge 3 ]; then
  ok "Dashboard: ${ACTIVE}/${TOTAL} nodes active"
else
  fail "Dashboard" "$CONT"
fi

echo ""
echo "--- 8. Dashboard KPS Stats ---"
KPS_STATS=$(try_curl 5 http://127.0.0.1:3517/api/kps-stats)
CONN=$(echo "$KPS_STATS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('connected',False))" 2>/dev/null || echo "false")
if [ "$CONN" = "True" ]; then
  ok "Dashboard KPS stats: connected"
else
  fail "Dashboard KPS stats" "$KPS_STATS"
fi

echo ""
echo "============================================"
echo " Results: $PASS passed, $FAIL failed, $SKIP skipped"
echo "============================================"
exit $FAIL
