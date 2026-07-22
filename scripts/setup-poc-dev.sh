#!/bin/bash
set -euo pipefail

echo "=== ZKNetwork + Anon-RPC PoC Dev Setup ==="

command -v docker >/dev/null 2>&1 || { echo "Docker required"; exit 1; }
command -v go >/dev/null 2>&1 || { echo "Go required"; exit 1; }
command -v openssl >/dev/null 2>&1 || { echo "openssl required"; exit 1; }

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# 1. Build mixnet Docker image
echo "[1/5] Building mixnet Docker image..."
docker images zeros/mixnet-node:amd64 | grep -q amd64 || {
    echo "Building mixnet image (this takes 30-60 min on first run)..."
    docker build \
        --build-arg TARGETARCH=amd64 \
        --build-arg BUILDER_ARCH=amd64 \
        -t zeros/mixnet-node:amd64 \
        -f "$PROJECT_ROOT/Dockerfile.mixnet" \
        "$PROJECT_ROOT"
}

# 2. Generate mixnet configs
echo "[2/5] Generating mixnet configs..."
if [ ! -f "$PROJECT_ROOT/config/mixnet/pkidata/consortium.yaml" ]; then
    bash "$PROJECT_ROOT/scripts/gen-mixnet-configs.sh"
fi

# 3. Start mixnet stack
echo "[3/5] Starting mixnet stack..."
docker compose -f "$PROJECT_ROOT/docker-compose.yml" up -d
echo "Waiting for PKI consensus (60s)..."
sleep 60

# Verify mixnet is ready
echo "Checking mixnet status..."
docker ps | grep mix-

# 4. Build walletshield with KPS
echo "[4/5] Building walletshield with KPS..."
cd "$PROJECT_ROOT/walletshield"
go mod tidy 2>/dev/null || true
go build -trimpath -o walletshield-kps .

# Build the worker bundle
echo "Building Anon-RPC worker bundle..."
cd "$PROJECT_ROOT/zkn-anon-rpc-worker"
npm install --silent 2>/dev/null || true
npx esbuild zkn-walletshield-worker.js --bundle --minify --outfile=dist/worker.js 2>/dev/null || {
    echo "esbuild not found, copying raw worker bundle"
    cp zkn-walletshield-worker.js dist/worker.js
}

cd "$PROJECT_ROOT"

# 5. Start walletshield with KPS
echo "[5/5] Starting walletshield with KPS..."
cd walletshield
./walletshield-kps \
    -config "$PROJECT_ROOT/config/walletshield/config.toml" \
    -listen 127.0.0.1:9200 \
    -kps_listen 0.0.0.0:9201 \
    -log_level INFO &
WS_PID=$!
echo "walletshield PID: $WS_PID"
cd "$PROJECT_ROOT"

# Print connection info
HOST_IP=$(hostname -I | awk '{print $1}')
echo ""
echo "=== PoC Ready ==="
echo "walletshield: PID $WS_PID"
echo "Boot URL:     http://127.0.0.1:9200/boot"
echo "KPS address:  http://127.0.0.1:9201 (certhash from /boot)"
echo ""
echo "Test HTTP proxy: curl -X POST http://127.0.0.1:9200/ethereum -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_blockNumber\",\"params\":[],\"id\":1}'"
echo ""
echo "To test via KPS from browser:"
echo "  https://privacy-ethereum.github.io/anon-rpc/demo/"
echo ""
echo "To stop: kill $WS_PID && docker compose -f docker-compose.yml down"
echo "To clean: rm -rf config/mixnet/pkidata config/mixnet/auth* config/mixnet/mix*"
