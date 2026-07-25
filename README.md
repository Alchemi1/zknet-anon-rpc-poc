# ZKNetwork + Anon-RPC Proof of Concept

Anon-RPC wallets route Ethereum JSON-RPC through the ZKNetwork Katzenpost mixnet. This repo builds the entire Katzenpost mixnet from source, runs it in Docker, and provides HTTP proxy and KPS transport for Ethereum RPC access. Includes a Tauri desktop dashboard for monitoring and control.

## Dashboard

A Tauri v2 desktop app (`dashboard/`) provides real-time monitoring and control:

```
┌─────────────────────────────────────────────────────────┐
│  ZKNetwork Mixnet Dashboard                    ● online │
├─────────────────────────────────────────────────────────┤
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ │
│  │dirauth│ │dirauth│ │dirauth│ │ mix1 │ │ mix2 │ │ mix3 │ │
│  │ ● Up  │ │ ● Up  │ │ ● Up  │ │ ● Up │ │ ● Up │ │ ● Up │ │
│  └──────┘ └──────┘ └──────┘ └──────┘ └──────┘ └──────┘ │
│  ┌──────┐ ┌─────────┐ ┌──────────┐                       │
│  │gateway│ │servicenode│ │  client  │                       │
│  │ ● Up  │ │  ● Up   │ │  ● Up   │                       │
│  └──────┘ └─────────┘ └──────────┘                       │
├─────────────────────────────────────────────────────────┤
│  PKI: Epoch 240546 ● Consensus OK                        │
│  HTTP Proxy Test: [Test Mixnet] → 0xad1673 (11342451)     │
│  Logs: [mix-servicenode ▼] 10:15:59 ...                   │
└─────────────────────────────────────────────────────────┘
```

### Build & Run

```bash
cd dashboard
npm install
npm run tauri build     # or: cargo build in src-tauri/
./src-tauri/target/debug/zknet-dashboard
```

## Architecture

```
curl → http-proxy-client:9205
  │
  └── thin client protocol (TCP 64332)
      │
      kpclientd
        │
        ├── Sphinx encrypt → gateway → mix1 → mix2 → mix3 → servicenode
        │                                                           │
        │                                              http-proxy-server (Kaetzchen plugin)
        │                                                           │
        │                                              https://ethereum-sepolia.publicnode.com
        │
        └── SURB decrypt ← gateway ← mix3 ← mix2 ← mix1 ← servicenode
```

### Docker Mixnet Stack

| Container | Role | Ports |
|-----------|------|-------|
| `mix-dirauth-1/2/3` | PKI directory authorities (voting consensus) | 30001-30003 |
| `mix-1/2/3` | Sphinx mix nodes (packet mixing & forwarding) | 30011,30014,30017 |
| `mix-gateway` | Gateway node (client entry point) | 30004 |
| `mix-servicenode` | Service node with http_proxy Kaetzchen plugin | 30010 |
| `mix-client` | kpclientd daemon (thin client endpoint) | 64332 |

### Two Access Paths

| Path | Description |
|------|-------------|
| **HTTP proxy** (this readme) | `http-proxy-client` listens on a local port, sends request through mixnet to `http-proxy-server` plugin on servicenode, which makes the actual HTTP request |
| **KPS** (future) | WalletShield connects via KPS transport, routes through same mixnet |

## Prerequisites

- Docker + Docker Compose (v2+)
- ~4GB free disk, ~30min for initial build
- Go 1.24+ (optional, for walletshield)

## Quick Start

```bash
# 1. Build the mixnet Docker image (~30 min, compiles Go + RocksDB)
docker build -t zeros/mixnet-node:amd64 -f Dockerfile.mixnet .

# 2. Start the full mixnet
docker compose up -d

# 3. Wait for PKI consensus (~90s)
docker logs mix-client -f
# Look for: "PKI doc available"

# 4. Start the HTTP proxy client
docker exec mix-client \
  /usr/local/bin/http-proxy-client \
  -cfg /var/lib/katzenpost/client/thinclient.toml \
  -port 9205 -ep http_proxy -log_level DEBUG &

# 5. Query through the mixnet
curl -X POST http://127.0.0.1:9205/ \
  -H "Content-Type: application/json" \
  -H "Host: ethereum-sepolia.publicnode.com" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'

# Expected response (block number varies):
# {"jsonrpc":"2.0","result":"0x...","id":1}
```

Response takes ~60-120 seconds (mixnet round-trip time including Poisson scheduling, Sphinx delay, and actual HTTP request).

## Project Structure

```
zknet-anon-rpc-poc/
├── Dockerfile.mixnet              # Katzenpost build: 7 binaries from source
├── docker-compose.yml             # 9-service mixnet stack
├── katzenpost/                    # Katzenpost upstream source (v0.0.97)
│   └── cmd/
│       ├── server/                # Mix/gateway/service node binary
│       ├── dirauth/               # Directory authority binary
│       ├── courier/               # Courier Kaetzchen plugin
│       ├── echo-plugin/           # Echo Kaetzchen plugin
│       ├── http-proxy-server/     # HTTP proxy Kaetzchen plugin (servicenode side)
│       ├── http-proxy-client/     # HTTP proxy client (local side)
│       ├── kpclientd/             # Client daemon with thin client protocol
│       └── fetch/                 # Network topology fetcher
├── config/mixnet/
│   ├── auth1-3/authority.toml     # Authority configs (PKI voting)
│   ├── mix1-3/katzenpost.toml     # Mix node configs
│   ├── gateway1/katzenpost.toml   # Gateway config
│   ├── servicenode1/
│   │   ├── katzenpost.toml        # Service node config (with Kaetzchen plugins)
│   │   └── http_proxy_config.toml # Upstream network mappings
│   ├── client/
│   │   ├── client.toml            # kpclientd config (pinned gateway)
│   │   └── thinclient.toml        # Thin client dial config
│   └── courier/courier.toml       # Courier plugin config
├── patches/
│   └── fix-decoy-sender-nil-pointer.patch
├── test-via-curl.sh               # HTTP proxy test
├── test-via-mixnet.js             # KPS path test
└── test-boot.sh                   # Boot endpoint test
```

## Detailed Setup

### Build the Image

```bash
docker build -t zeros/mixnet-node:amd64 -f Dockerfile.mixnet .
```

Builds 7 binaries from `katzenpost/cmd/*/`:
- `dirauth`, `server`, `courier`, `echo-plugin`
- `http-proxy-server`, `http-proxy-client`, `kpclientd`, `fetch`

### Start the Mixnet

```bash
docker compose up -d
# 9 containers: 3 dirauth + 3 mix + 1 gateway + 1 servicenode + 1 client
```

### Monitor PKI Consensus

```bash
docker logs mix-dirauth-1 -f
# Wait for: "SUCCESS! Achieved threshold consensus for epoch N"

docker logs mix-client -f
# Wait for: "PKI doc available"
```

PKI epochs run every 20 minutes with a ~12-minute voting cycle. If you restart any container, mix keys are regenerated and you must wait for the next epoch for keys to re-sync (see Troubleshooting).

### Test the HTTP Proxy

```bash
# Start http-proxy-client on the client container
docker exec -d mix-client \
  /usr/local/bin/http-proxy-client \
  -cfg /var/lib/katzenpost/client/thinclient.toml \
  -port 9205 -ep http_proxy -log_level DEBUG

# Send an Ethereum JSON-RPC request through the mixnet
# Must set Host header to the target Ethereum endpoint
curl -X POST http://127.0.0.1:9205/ \
  -H "Content-Type: application/json" \
  -H "Host: ethereum-sepolia.publicnode.com" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
```

## Mix Key Persistence

By default, Katzenpost regenerates mix keys on every restart. This causes Sphinx packet decryption failures (MAC mismatch) for one full epoch after any node restart. This repo patches the source to **persist mix keys to disk**.

Key files are stored at `<dataDir>/mixkey-<epoch>.key`:
- Format: 1-byte type (0=NIKE, 1=KEM) + public key bytes + private key bytes
- NIKE: 65 bytes total (type + 32B pub + 32B priv)
- Generated on first startup, loaded from disk on subsequent starts

This means: restart any node at any time without breaking the network for the next epoch.

## Patches Applied

| Patch | File | Description |
|-------|------|-------------|
| `fix-decoy-sender-nil-pointer.patch` | `client/sender.go` | Nil-check `rates` before dereferencing |
| nil rate check | `client/sender.go` | `if rates == nil \|\| rates.messageOrLoop <= 0` |
| socket panic→log | `server/cborplugin/socket.go:91` | `panic(err)` → `c.log.Fatalf(...)` |
| mix key persistence | `server/internal/mixkey/mixkey.go` | `Init()` function to load keys from disk |
| mix key persistence | `server/internal/mixkeys/mixkeys.go` | `saveKeyToDisk()` / `loadKeyFromDisk()` methods |
| HTTP proxy URL fix | `cmd/http-proxy-server/main.go` | Handle path-only URLs by constructing full URL from Host header |
| HTTP proxy client fix | `cmd/http-proxy-client/main.go` | Set URL scheme to https before dumping request |

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `kpclientd: connection refused` | Mixnet not ready | Wait 60s for PKI consensus |
| `Sphinx MAC mismatch` | Node restarted, mix keys regenerated | Wait for next epoch (~20 min) |
| `unsupported protocol scheme ""` | http-proxy-server got path-only URL | Set `Host` header in curl, or use fixed http-proxy-client |
| `context deadline exceeded` | Mixnet round-trip > 60s | Retry; mixnet delays can be long |
| `PKI doc not available` | Dirauths not in consensus | Check `docker logs mix-dirauth-1` for voting errors |
| `ERROR: Layers 5 exceeds maximum` | Too many topology layers | Keep `[Debug] Layers ≤ 3` |
| `Document contains multiple entries` | Duplicate nodes in topology | Don't reuse mix node identifiers across layers |

## References

- Anon-RPC SPEC: https://github.com/privacy-ethereum/anon-rpc/blob/main/SPEC.md
- Anon-RPC demo: https://privacy-ethereum.github.io/anon-rpc/demo/
- Katzenpost: https://github.com/katzenpost/katzenpost
- KPS library: https://github.com/privacy-ethereum/kps
