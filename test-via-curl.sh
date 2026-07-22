#!/bin/bash
# Test walletshield HTTP proxy via mixnet
set -euo pipefail

HOST="${1:-127.0.0.1:9200}"
METHOD="${2:-eth_blockNumber}"

echo "=== Testing walletshield HTTP proxy ==="
echo "Endpoint: http://$HOST/ethereum"
echo "Method:   $METHOD"
echo ""

curl -s -X POST "http://$HOST/ethereum" \
  -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"$METHOD\",\"params\":[],\"id\":1}" | jq .
