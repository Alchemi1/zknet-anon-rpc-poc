#!/bin/bash
# Test walletshield boot endpoint
set -euo pipefail

HOST="${1:-127.0.0.1:9200}"

echo "=== Boot Endpoint ==="
curl -s "http://$HOST/boot" | jq .

echo ""
echo "=== Worker Bundle Hash ==="
curl -s "http://$HOST/get-worker" | openssl dgst -sha256 -binary | xxd -p -c 64
echo ""
