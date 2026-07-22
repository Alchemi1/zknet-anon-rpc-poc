#!/bin/bash
set -euo pipefail

echo "=== Deploy ZKNWalletShield Specifier Contract ==="

if [ -z "${PRIVATE_KEY:-}" ]; then
    echo "Error: PRIVATE_KEY environment variable not set"
    echo "Usage: PRIVATE_KEY=0x... ./deploy.sh"
    exit 1
fi

if [ -z "${RPC_URL:-}" ]; then
    RPC_URL="https://ethereum-sepolia.publicnode.com"
    echo "Using default RPC: $RPC_URL"
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Hash the worker bundle
echo "Computing worker bundle hash..."
HASH=$(openssl dgst -sha256 -binary dist/worker.js | xxd -p -c 64)
echo "Worker hash: 0x$HASH"

# Deploy contract
echo "Deploying ZKNWalletShield..."
DEPLOY_OUTPUT=$(forge create ZKNWalletShield \
    --constructor-args "0x$HASH" \
    '["http://127.0.0.1:9200/get-worker"]' \
    --rpc-url "$RPC_URL" \
    --private-key "$PRIVATE_KEY" \
    2>&1)

echo "$DEPLOY_OUTPUT"

# Extract deployed address
CONTRACT_ADDR=$(echo "$DEPLOY_OUTPUT" | grep -oP 'Deployed to: \K(0x[a-fA-F0-9]+)' || true)
if [ -n "$CONTRACT_ADDR" ]; then
    echo ""
    echo "=== Deployment Complete ==="
    echo "Contract address: $CONTRACT_ADDR"
    echo "Worker hash:      0x$HASH"
    echo ""
    echo "Update walletshield main.go with:"
    echo "  contractAddr = \"$CONTRACT_ADDR\""
fi
