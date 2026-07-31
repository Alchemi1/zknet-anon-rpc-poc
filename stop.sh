#!/bin/bash
# ZKNetwork / Anon-RPC — graceful spin-down
set -euo pipefail

echo "=== ZKNetwork Anon-RPC Spin-Down ==="

echo ""
echo "--- Stopping host kps-monitor ---"
if pkill -f "kps-monitor.*-http :9206" 2>/dev/null; then
  echo "  [OK] kps-monitor stopped"
else
  echo "  [OK] kps-monitor not running"
fi

echo ""
echo "--- Stopping walletshield-kps host process ---"
if pkill -f "walletshield-kps.*-kps_listen" 2>/dev/null; then
  echo "  [OK] walletshield-kps stopped"
else
  echo "  [OK] walletshield-kps not running"
fi

echo ""
echo "--- Stopping Docker mixnet (9 services) ---"
cd "$(dirname "$0")"
if docker compose down --timeout 30 2>&1; then
  echo "  [OK] mixnet containers stopped"
else
  echo "  [WARN] docker compose down had issues — some containers may need manual cleanup"
fi

echo ""
echo "=== Spin-down complete ==="
echo "To restart:  ./start.sh"
