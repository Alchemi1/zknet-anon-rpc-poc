# ZKNetwork + Anon-RPC Proof of Concept

Anon-RPC wallets route Ethereum JSON-RPC through the ZKNetwork Katzenpost mixnet. This repo builds the entire Katzenpost mixnet from source, runs it in Docker, and provides **three access paths** for Ethereum RPC:

- **HTTP proxy** — `http-proxy-client` on `:9205` accepts plain HTTP, forwards through mixnet
- **WalletShield + KPS** — Go binary on `:9200` adds KPS listener on `:9201`, `/boot` + `/get-worker` endpoints
- **KPS client CLI** — Go CLI (`kps-client/`) dials the KPS listener over QUIC and sends RPC through the mixnet

A **Python web dashboard** (`:3517`) provides real-time monitoring including **KPS transport stats** and a **live balance watcher**.

## Architecture

```
── HTTP proxy path ──
curl → http-proxy-client:9205
        │
        └── thin client protocol (TCP 64332)
            │
            kpclientd
              │
              ├── Sphinx encrypt → gateway → mix1 → mix2 → mix3 → servicenode
              │                                                       │
              │                                          http-proxy-server (Kaetzchen plugin)
              │                                                       │
              │                                          https://ethereum-sepolia.publicnode.com
              │
              └── SURB decrypt ← gateway ← mix3 ← mix2 ← mix1 ← servicenode

── WalletShield / KPS path ──
KPS client → walletshield-kps:9201 (QUIC KPS)
                  │
                  ├── /boot         → KPS address
                  ├── /get-worker   → Anon-RPC worker bundle
                  ├── /             → HTTP proxy through mixnet (CBOR-wrapped)
                  │
                  └── thin client protocol → kpclientd → mixnet → servicenode

── Monitoring path ──
Browser → Python dashboard:3517
              │
              ├── /api/kps-stats → kps-monitor:9206/stats
              │                       └── persistent KPS connection → walletshield → mixnet → RPC
              ├── /api/kps-rpc   → kps-monitor:9206/rpc  (on-demand RPC via KPS)
              └── /api/test      → http-proxy-client:9205 (direct HTTP proxy test)
```

### Docker Mixnet Stack

| Container | Role | Ports |
|-----------|------|-------|
| `mix-dirauth-1/2/3` | PKI directory authorities (voting consensus) | 30001-30003 |
| `mix-1/2/3` | Sphinx mix nodes (packet mixing & forwarding) | 30011,30014,30017 |
| `mix-gateway` | Gateway node (client entry point) | 30004 |
| `mix-servicenode` | Service node with http_proxy Kaetzchen plugin | 30010 |
| `mix-client` | kpclientd daemon + walletshield + http-proxy-client + kps-monitor | 64332,9200,9201,9205,9206 |

All containers use `network_mode: host`. Docker security: `no-new-privileges`, `cap_drop: [ALL]`, selective `cap_add`.

## Quick Start

```bash
# 0. Ensure port 9200 is free (stop host walletshield if running)
systemctl --user stop walletshield.service 2>/dev/null || true

# 1. Build the mixnet Docker image (~30 min)
docker build -t zeros/mixnet-node:amd64 -f Dockerfile.mixnet .

# 2. Build the walletshield binary
docker build -t walletshield -f Dockerfile.walletshield.local .
docker create --name ws walletshield && docker cp ws:/usr/local/bin/walletshield walletshield/walletshield-kps && docker rm ws

# 3. Build kps-client and kps-monitor
docker run --rm -v $(pwd)/kps-client:/src -w /src golang:latest go build -o kps-client .
docker run --rm -v $(pwd)/kps-monitor:/src -w /src golang:latest go build -o kps-monitor .

# 4. Start the full stack
./start.sh
# → Dashboard at http://127.0.0.1:3517
```

Or run individual steps:

```bash
# Start containers
docker compose up -d

# Wait for PKI consensus (~90s)
docker logs mix-client -f
# Look for: "PKI doc available"

# Start services inside mix-client
docker exec -d mix-client /usr/local/bin/http-proxy-client -cfg /var/lib/katzenpost/client/thinclient.toml -port 9205 -ep http_proxy -log_level DEBUG
docker exec -d mix-client /usr/local/bin/walletshield-kps -config /var/lib/katzenpost/client/thinclient.toml -listen 127.0.0.1:9200 -kps_listen 0.0.0.0:9201 -log_level INFO
docker exec -d mix-client /usr/local/bin/kps-monitor -boot http://127.0.0.1:9200 -http :9206 -interval 30s

# Start dashboard
cd dashboard && python3 server.py &
```

## Dashboard

The web dashboard at `http://127.0.0.1:3517` provides:

| Card | Data | Refresh |
|------|------|---------|
| **Node Grid** | 9 container statuses (Up/Down/Restarting) | 15s |
| **PKI Consensus** | Current epoch, consensus status, last consensus time | 15s |
| **Anon-RPC / KPS** | Walletshield status, KPS address, worker bundle info | 15s |
| **KPS Transport** | Connection status, KPS address, last/min/avg/max RTT, success rate, probe counts, uptime, RTT history bar chart | 15s |
| **KPS RPC** | Send arbitrary `eth_*` methods via KPS with result display | on-demand |
| **Balance Watcher** | Live ETH balance for any address, polled every 10s via KPS, balance history chart | 10s polling |
| **HTTP Proxy Test** | Send `eth_blockNumber` through mixnet, shows RTT + result | on-demand |
| **Container Logs** | Tail 30 lines for any container | 30s |

APIs:
- `GET /api/containers` — Docker container status
- `GET /api/epoch` — PKI epoch, consensus status
- `GET /api/anonrpc` — WalletShield, KPS, worker info
- `GET /api/kps-stats` — KPS monitor stats (proxied from `:9206/stats`)
- `POST /api/kps-rpc` — Send arbitrary JSON-RPC via KPS (proxied from `:9206/rpc`)
- `POST /api/test` — Test mixnet HTTP proxy round-trip
- `GET /api/logs/{container}` — Container logs

## KPS Client CLI

A Go CLI that demonstrates the full KPS transport path:

```bash
# Get KPS address
curl -s http://127.0.0.1:9200/boot

# Run KPS client
./kps-client/kps-client -boot http://127.0.0.1:9200 -method eth_blockNumber

# Custom RPC
./kps-client/kps-client -boot http://127.0.0.1:9200 -method eth_getBalance -rpc-host ethereum-sepolia.publicnode.com
```

Flow: `kps-client → kps.Dial (QUIC) → walletshield KPS :9201 → thin client → kpclientd → mixnet → servicenode → http-proxy-server → Ethereum Sepolia RPC`

## KPS Monitor

A persistent monitoring service that maintains a KPS connection and exposes HTTP endpoints:

```bash
# Start
docker exec -d mix-client /usr/local/bin/kps-monitor -boot http://127.0.0.1:9200 -http :9206 -interval 30s

# Get stats
curl http://127.0.0.1:9206/stats

# Send on-demand RPC via KPS
curl -X POST http://127.0.0.1:9206/rpc \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'

# Watch a balance
curl -X POST http://127.0.0.1:9206/rpc \
  -d '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045","latest"],"id":1}'
```

## KPS SendTx (anonymous transaction broadcast)

Signs an Ethereum transaction **locally** and broadcasts it through the mixnet via `kps-monitor /rpc` — the private key never leaves your machine and the recipient node never sees your IP.

```bash
# Build
docker run --rm -v $(pwd)/kps-sendtx:/src -w /src golang:latest go build -o kps-sendtx .

# Broadcast with a funded key (prints tx hash on Sepolia)
./kps-sendtx/kps-sendtx -pk <hex_private_key> -to 0x... -value 1000000000000000

# Without a key: generates a random key, signs, and proves end-to-end delivery
# (node replies "insufficient funds" — proof the request reached the node via the mixnet)
./kps-sendtx/kps-sendtx

# Options
#   -pk        private key hex (default: random)
#   -to        recipient address
#   -value     wei to send
#   -chain-id  EIP-155 chain id (default 11155111 = Sepolia)
#   -rpc       kps-monitor /rpc endpoint
```

Flow: `local signing (secp256k1 + keccak256, EIP-155) → kps-monitor /rpc → KPS :9201 → thin client → kpclientd → mixnet → http-proxy-server → eth_sendRawTransaction`. The nonce and gas price are also fetched **through the mixnet**, so no metadata leaks outside the private channel.

### Dashboard: Private Broadcast card

The dashboard also has a **Private Broadcast** card that signs **in the browser** using ethers — the private key never leaves the page. It fetches nonce/gas/chain-id via `/api/kps-rpc`, signs locally, and sends only the signed raw tx through the mixnet:

1. Open the dashboard (`http://127.0.0.1:3517`) → **Private Broadcast** card.
2. A public demo key is pre-filled (0 balance, used for the standard demo — proves end-to-end delivery via the "insufficient funds" node response). Replace it with a funded key to confirm a real tx hash.
3. Enter a recipient and value in wei.
4. Click **Sign & Broadcast via KPS** — result shows the tx hash with a link to Etherscan, or the node's error (which still proves the tx reached the node via the mixnet).

### Dashboard: Mempool / Contract / Tx demo cards

Four additional demo cards, all routed anonymously through the mixnet via `/api/kps-rpc`:

| Card | RPC method | What it shows |
|------|-----------|----------------|
| **Mempool Watcher** | `eth_getBlockTransactionCountByNumber("pending")` | Live count of pending txs, polled every 10s with a history chart — watch the mempool without being watched |
| **Contract Reader** | `eth_call` | Read any contract's `totalSupply` / `symbol` / `name` / `decimals` / `balanceOf` anonymously (defaults to USDC on Sepolia) |
| **Tx Tracker** | `eth_getTransactionByHash` | Look up a tx hash and see block, from/to, value, gas |
| **Tx Simulator** | `eth_estimateGas` | Estimate gas for an unsent tx (from/to/value/data) |

> Note: the mixnet caps reply payloads at ~2000 bytes (`UserForwardPayloadLength`), so large responses (full blocks, transaction receipts) don't fit. These cards use small-response methods that traverse the mixnet reliably.

Build after editing the frontend:
```bash
cd dashboard && npx vite build
```

## Demo Script

```bash
./demo.sh
```

Tests all 9 transport paths with per-step timeouts:
1. Walletshield boot endpoint
2. HTTP proxy via walletshield (`:9200`)
3. Direct HTTP proxy (`:9205`)
4. KPS client CLI
5. KPS monitor stats
6. KPS RPC on-demand
6b. Anonymous tx broadcast (`eth_sendRawTransaction` via mixnet, with or without `SEND_PK`)
6c. Mempool watcher (`eth_getBlockTransactionCountByNumber("pending")`)
6d. Contract reader (`eth_call` symbol on USDC)
6e. Tx simulator (`eth_estimateGas`)
7. Dashboard container status
8. Dashboard KPS stats

To run the broadcast with a real funded key: `SEND_PK=<hex> ./demo.sh`

## Security Audit

A ZKN security audit MCP server is available:

```bash
cd /path/to/zkn-security-server
npm install

# Full audit
./zkn-audit scan /path/to/zknet-anon-rpc-poc

# Category-specific
./zkn-audit scan /path/to/zknet-anon-rpc-poc --category mixnet --min-severity high

# Rules info
./zkn-audit list-rules
./zkn-audit rule ZKN-MX-001

# Serve as MCP server
./zkn-audit serve
```

60 rules across 7 categories (smart contracts, ZK crypto, circuits, mixnet, infra, dApp, supply chain). Evidence-gated (confirmed/likely/plausible/unconfirmed). Auto-fix patching for Solidity.

## Project Structure

```
zknet-anon-rpc-poc/
├── start.sh                        # All-in-one startup
├── stop.sh                         # Graceful spin-down (kills host processes + docker compose down)
├── demo.sh                         # Full-stack demo script
├── Dockerfile.mixnet              # Katzenpost build
├── Dockerfile.mixnet-proxy        # Mixnet proxy build (arm64)
├── Dockerfile.walletshield        # WalletShield build (arm64)
├── Dockerfile.walletshield.local  # WalletShield local build (amd64)
├── docker-compose.yml             # 9-service mixnet stack
├── MIXNET_TUNING.md               # Mixnet parameter tuning guide
├── katzenpost/                    # Katzenpost upstream source (submodule)
│   └── cmd/
│       ├── server/                # Mix/gateway/service node binary
│       ├── dirauth/               # Directory authority binary
│       ├── courier/               # Courier Kaetzchen plugin
│       ├── http-proxy-server/     # HTTP proxy Kaetzchen plugin
│       ├── http-proxy-client/     # HTTP proxy client
│       ├── kpclientd/             # Client daemon
│       └── fetch/                 # Network topology fetcher
├── walletshield/
│   ├── main.go                    # WalletShield: HTTP + KPS + mixnet proxy
│   ├── go.mod / go.sum
│   ├── walletshield-kps           # Built binary
│   ├── walletshield.py            # Python wrapper (fallback)
│   └── worker.js                  # Anon-RPC worker bundle placeholder
├── kps-client/                    # KPS demo client (Go)
│   ├── main.go                    # CLI: dial KPS, send RPC, display result
│   ├── go.mod / go.sum
│   └── kps-client                 # Built binary
├── kps-monitor/                   # KPS monitoring service (Go)
│   ├── main.go                    # Persistent KPS connection + /stats + /rpc
│   ├── go.mod / go.sum
│   └── kps-monitor                # Built binary
├── kps-sendtx/                    # Anonymous tx broadcast tool (Go)
│   ├── main.go                    # Local EIP-155 signing + broadcast via mixnet
│   ├── main_test.go               # Address-derivation + RLP encoding tests
│   ├── go.mod / go.sum
│   └── kps-sendtx                 # Built binary
├── dashboard/
│   ├── server.py                  # Python web dashboard
│   ├── index.html                 # Frontend HTML
│   ├── src/main.ts                # TypeScript frontend
│   └── dist/                      # Built frontend assets
├── zkn-anon-rpc-worker/
│   ├── zkn-walletshield-worker.js # Anon-RPC worker source
│   ├── ZKNWalletShield.sol        # Specifier contract
│   └── dist/worker.js             # Built worker bundle
├── config/mixnet/
│   ├── auth1-3/authority.toml     # Authority configs
│   ├── mix1-3/katzenpost.toml     # Mix node configs
│   ├── gateway1/katzenpost.toml   # Gateway config
│   ├── servicenode1/
│   │   ├── katzenpost.toml        # Service node config
│   │   └── http_proxy_config.toml # Upstream RPC mappings
│   ├── client/client.toml         # kpclientd config
│   └── client/thinclient.toml     # Thin client dial config
├── patches/
│   └── fix-decoy-sender-nil-pointer.patch
└── test-*.sh / test-*.js          # Test scripts
```

## Mixnet Tuning

See `MIXNET_TUNING.md` for current parameters. Key levers:

| Parameter | Default | Tuned | Effect |
|-----------|---------|-------|--------|
| `LambdaP` (Poisson rate) | 0.001 | **0.008** | ~125ms mean scheduling delay (was ~1000ms) |
| `UnwrapDelay` (per hop) | 250ms | **50ms** | Sphinx unwrap |
| `GatewayDelay` | 500ms | **100ms** | Gateway processing |
| `ServiceDelay` | 500ms | **100ms** | Service node processing |
| `KaetzchenDelay` | 750ms | **200ms** | Plugin processing |
| `DecoySlack` | 15000ms | **5000ms** | Decoy timing |

## Security

Docker hardening applied:
- Non-root `app` user in all runtime images
- `security_opt: no-new-privileges:true`
- `cap_drop: [ALL]` with selective `cap_add: [NET_BIND_SERVICE, NET_ADMIN, NET_RAW]`
- Solidity pragma locked to `0.8.28`
- Mixnet config files are local-only (not deployed to production)

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `connection refused` on :9200 | walletshield not running in container | `docker exec -d mix-client /usr/local/bin/walletshield-kps ...` |
| `Read: timeout` | Mixnet slow or no consensus | Wait for epoch transition (~20 min after restart) |
| `OpenStream: timeout` | KPS connection lost | kps-monitor auto-reconnects on failure |
| `http-proxy-client` dead | Started via `&` not `docker exec -d` | Use `start.sh` or `docker exec -d` |
| `PKI doc not available` | Dirauths not in consensus | Check `docker logs mix-dirauth-1` for voting errors |
| Port 9200 in use | Host walletshield from `com.zkn-client.app` | `systemctl --user stop walletshield.service` |

## References

- Anon-RPC: https://github.com/privacy-ethereum/anon-rpc
- Anon-RPC demo: https://privacy-ethereum.github.io/anon-rpc/demo/
- Katzenpost: https://github.com/katzenpost/katzenpost
- KPS library: https://github.com/privacy-ethereum/kps
- ZKN Security Audit: (local) `mcp-servers/zkn-security-server`
