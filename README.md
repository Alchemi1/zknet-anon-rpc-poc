# ZKNetwork + Anon-RPC Proof of Concept

Anon-RPC wallets route Ethereum JSON-RPC through the ZKNetwork Katzenpost mixnet. This repo builds the entire Katzenpost mixnet from source, runs it in Docker, and provides **two access paths** for Ethereum RPC:

- **HTTP proxy** — `http-proxy-client` on `:9205` accepts plain HTTP, forwards through mixnet
- **WalletShield + KPS** — Go binary on `:9200` adds KPS WebRTC listener on `:9201`, `/boot` + `/get-worker` endpoints for Anon-RPC client discovery

A **Python web dashboard** (`:3517`) provides real-time monitoring, and a **Tauri v2 desktop dashboard** is also available for local GUI use.

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
KPS client → walletshield-kps:9201 (WebRTC KPS)
                 │
                 ├── /boot         → KPS address + worker info
                 ├── /get-worker   → Anon-RPC worker bundle
                 ├── /             → HTTP proxy through mixnet (CBOR-wrapped)
                 │
                 └── thin client protocol (TCP 64332)
                     │
                     kpclientd → gateway → mix1 → mix2 → mix3 → servicenode

── Dashboards ──
Browser → Python web dashboard:3517
Browser → Tauri desktop dashboard (standalone binary)
```

### Docker Mixnet Stack

| Container | Role | Ports |
|-----------|------|-------|
| `mix-dirauth-1/2/3` | PKI directory authorities (voting consensus) | 30001-30003 |
| `mix-1/2/3` | Sphinx mix nodes (packet mixing & forwarding) | 30011,30014,30017 |
| `mix-gateway` | Gateway node (client entry point) | 30004 |
| `mix-servicenode` | Service node with http_proxy Kaetzchen plugin | 30010 |
| `mix-client` | kpclientd daemon + walletshield + http-proxy-client | 64332,9200,9201,9205 |

All containers use `network_mode: host` — services are available on `127.0.0.1` directly.

## Dashboards

### Python Web Dashboard (`:3517`)

A standalone Python HTTP server that monitors the full mixnet stack.

```
┌───────────────────────────────────────────────────────────┐
│  ZKNetwork Mixnet Dashboard                    ● online   │
├───────────────────────────────────────────────────────────┤
│  ┌───────┐ ┌───────┐ ┌───────┐ ┌──────┐ ┌──────┐ ┌──────┐ │
│  │dirauth│ │dirauth│ │dirauth│ │ mix1 │ │ mix2 │ │ mix3 │ │
│  │ ● Up  │ │ ● Up  │ │ ● Up  │ │ ● Up │ │ ● Up │ │ ● Up │ │
│  └───────┘ └───────┘ └───────┘ └──────┘ └──────┘ └──────┘ │
│  ┌───────┐ ┌───────────┐ ┌──────────┐                     │
│  │gateway│ │servicenode│ │  client  │                     │
│  │ ● Up  │ │    ● Up   │ │  ● Up    │                     │
│  └───────┘ └───────────┘ └──────────┘                     │
├───────────────────────────────────────────────────────────┤
│  PKI: Epoch 240546 ● Consensus OK                         │
│  Anon-RPC: ws=online worker=built kps=0.0.0.0:9201:uEiA…  │
│  HTTP Proxy Test: [Test Mixnet] → 0xad1673 (11342451)     │
│  Logs: [mix-servicenode ▼] 10:15:59 ...                   │
└───────────────────────────────────────────────────────────┘
```

**Run:**
```bash
cd dashboard
python3 server.py
# → http://127.0.0.1:3517
```

APIs:
- `GET /` — HTML page with live-updating panels
- `GET /api/containers` — Docker container status (9 containers)
- `GET /api/pki` — PKI epoch, consensus, service node count
- `GET /api/anonrpc` — WalletShield status, KPS address, worker bundle hash
- `GET /api/probe` — Test mixnet round-trip with `eth_blockNumber`
- `GET /api/logs?container=<name>&lines=100` — Container logs

### Tauri Desktop Dashboard

A cross-platform desktop app built with Tauri v2 + Rust + TypeScript.

```bash
cd dashboard
npm install
npm run tauri build
./src-tauri/target/debug/zknet-dashboard
```

## WalletShield (`walletshield-kps`)

The Go binary that provides KPS transport and HTTP proxy through the mixnet.

**Endpoints:**
| Endpoint | Port | Description |
|----------|------|-------------|
| `HTTP API` | `:9200` | `/boot`, `/get-worker`, `/` (HTTP proxy via mixnet) |
| `KPS Listener` | `:9201` | WebRTC KPS transport for Anon-RPC clients |

**Run:**
```bash
# Start inside mix-client container
docker exec -d mix-client \
  /usr/local/bin/walletshield-kps \
  -config /var/lib/katzenpost/client/thinclient.toml \
  -listen 127.0.0.1:9200 \
  -kps_listen 0.0.0.0:9201 \
  -log_level INFO

# Test /boot (KPS address + worker info)
curl http://127.0.0.1:9200/boot

# Test /get-worker (worker bundle)
curl -o worker.js http://127.0.0.1:9200/get-worker

# Test HTTP proxy through mixnet
curl -X POST http://127.0.0.1:9200/ \
  -H "Content-Type: application/json" \
  -H "Host: ethereum-sepolia.publicnode.com" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
```

**Build from source:**
```bash
docker build -t walletshield -f Dockerfile.walletshield.local .
docker create --name ws walletshield
docker cp ws:/usr/local/bin/walletshield walletshield/walletshield-kps
docker rm ws
```

## Prerequisites

- Docker + Docker Compose (v2+)
- ~4GB free disk, ~30min for initial mixnet build

## Quick Start

```bash
# 0. Ensure port 9200 is free (stop host walletshield if running)
systemctl --user stop walletshield.service 2>/dev/null || true

# 1. Build the mixnet Docker image (~30 min)
docker build -t zeros/mixnet-node:amd64 -f Dockerfile.mixnet .

# 2. Start the full mixnet (9 containers)
docker compose up -d

# 3. Wait for PKI consensus (~90s)
docker logs mix-client -f
# Look for: "PKI doc available"

# 4. Start HTTP proxy client and walletshield
docker exec -d mix-client \
  /usr/local/bin/http-proxy-client \
  -cfg /var/lib/katzenpost/client/thinclient.toml \
  -port 9205 -ep http_proxy -log_level DEBUG

docker exec -d mix-client \
  /usr/local/bin/walletshield-kps \
  -config /var/lib/katzenpost/client/thinclient.toml \
  -listen 127.0.0.1:9200 \
  -kps_listen 0.0.0.0:9201 \
  -log_level INFO

# 5. Start the web dashboard
cd dashboard && python3 server.py &
# → http://127.0.0.1:3517

# 6. Test via HTTP proxy (direct)
curl -X POST http://127.0.0.1:9205/ \
  -H "Content-Type: application/json" \
  -H "Host: ethereum-sepolia.publicnode.com" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'

# 7. Test via walletshield
curl -X POST http://127.0.0.1:9200/ \
  -H "Content-Type: application/json" \
  -H "Host: ethereum-sepolia.publicnode.com" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'

# 8. Test walletshield boot
curl http://127.0.0.1:9200/boot
```

Or use the all-in-one startup script:
```bash
./start.sh            # full startup with PKI wait
./start.sh --no-wait  # skip PKI wait (services already running)
```

**Note:** A host-level walletshield from `com.zkn-client.app` may occupy port 9200.
The startup script (`start.sh`) stops it automatically. If running manually,
run `systemctl --user stop walletshield.service` first, or disable it permanently with
`mv ~/.config/systemd/user/walletshield.service{,.disabled}`.

Expected response:
```json
{"jsonrpc":"2.0","result":"0xad251a","id":1}
```

Full round-trip takes ~5-60s (mixnet Poisson scheduling + Sphinx delays).

## Project Structure

```
zknet-anon-rpc-poc/
├── start.sh                        # All-in-one startup script (stops host walletshield, starts mixnet + services + dashboard)
├── Dockerfile.mixnet              # Katzenpost build: 8 binaries from source
├── Dockerfile.walletshield.local  # WalletShield Go binary build
├── docker-compose.yml             # 9-service mixnet stack (host networking)
├── katzenpost/                    # Katzenpost upstream source (v0.0.97, submodule)
│   └── cmd/
│       ├── server/                # Mix/gateway/service node binary
│       ├── dirauth/               # Directory authority binary
│       ├── courier/               # Courier Kaetzchen plugin
│       ├── echo-plugin/           # Echo Kaetzchen plugin
│       ├── http-proxy-server/     # HTTP proxy Kaetzchen plugin (servicenode side)
│       ├── http-proxy-client/     # HTTP proxy client (local side)
│       ├── kpclientd/             # Client daemon with thin client protocol
│       └── fetch/                 # Network topology fetcher
├── walletshield/
│   ├── main.go                    # WalletShield: HTTP + KPS listener + mixnet proxy
│   ├── go.mod / go.sum            # Go module deps (KPS v0.2.1, CBOR, local katzenpost)
│   ├── walletshield-kps           # Built binary (output of Dockerfile.walletshield.local)
│   ├── walletshield.py            # Lightweight Python wrapper (fallback)
│   └── worker.js                  # Anon-RPC worker bundle placeholder
├── dashboard/
│   ├── server.py                  # Python web dashboard server
│   ├── src/                       # TypeScript frontend (Vite build)
│   ├── dist/                      # Built frontend assets
│   └── src-tauri/                 # Tauri v2 Rust backend for desktop app
├── zkn-anon-rpc-worker/
│   ├── zkn-walletshield-worker.js # Anon-RPC worker source
│   └── dist/worker.js             # Built worker bundle
├── config/mixnet/
│   ├── auth1-3/authority.toml     # Authority configs (PKI voting)
│   ├── mix1-3/katzenpost.toml     # Mix node configs
│   ├── gateway1/katzenpost.toml   # Gateway config
│   ├── servicenode1/
│   │   ├── katzenpost.toml        # Service node config (http_proxy + courier + chat)
│   │   └── http_proxy_config.toml # Upstream network mappings
│   ├── client/
│   │   ├── client.toml            # kpclientd config (pinned gateway)
│   │   ├── thinclient.toml        # Thin client dial config
│   │   └── worker.js              # Anon-RPC worker bundle (copy)
│   └── courier/courier.toml       # Courier plugin config
├── patches/
│   └── fix-decoy-sender-nil-pointer.patch
├── MIXNET_TUNING.md               # Tuning guide for mixnet parameters
├── test-via-curl.sh               # Quick test script for walletshield
├── test-via-mixnet.js             # KPS path test (Node.js)
└── test-boot.sh                   # Boot endpoint test
```

## Mix Key Persistence

By default, Katzenpost regenerates mix keys on every restart. This causes Sphinx packet decryption failures (MAC mismatch) for one full epoch. This repo patches the source to **persist mix keys to disk**.

Key files stored at `<dataDir>/mixkey-<epoch>.key`:
- Format: 1-byte type (0=NIKE, 1=KEM) + public key bytes + private key bytes
- Generated on first startup, loaded from disk on subsequent starts
- Survives container restarts — no epoch wait required

## Patches Applied

| Patch | File | Description |
|-------|------|-------------|
| `fix-decoy-sender-nil-pointer.patch` | `client/sender.go` | Nil-check `rates` before dereferencing |
| nil rate check | `client/sender.go` | `if rates == nil || rates.messageOrLoop <= 0` |
| socket panic→log | `server/cborplugin/socket.go:91` | `panic(err)` → `c.log.Fatalf(...)` |
| mix key persistence | `server/internal/mixkey/mixkey.go` | `Init()` function to load keys from disk |
| mix key persistence | `server/internal/mixkeys/mixkeys.go` | `saveKeyToDisk()` / `loadKeyFromDisk()` methods |
| HTTP proxy URL fix | `cmd/http-proxy-server/main.go` | Handle path-only URLs, construct full URL from Host header |
| HTTP proxy client fix | `cmd/http-proxy-client/main.go` | Set URL scheme to "https" before dumping request |

## KPS Transport Test

```bash
# Install KPS QUIC client
npm install @kpstreams/quic-client

# Get KPS address from walletshield
KPS_ADDR=$(curl -s http://127.0.0.1:9200/boot | python3 -c "import json,sys;print(json.load(sys.stdin)['kpsAddr'])")

# Test KPS path
node test-via-mixnet.js $KPS_ADDR
```

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `kpclientd: connection refused` | Mixnet not ready | Wait 60s for PKI consensus |
| `Sphinx MAC mismatch` | Node restarted, mix keys regenerated | Wait for next epoch (~20 min) or use persisted keys |
| `unsupported protocol scheme ""` | http-proxy-server got path-only URL | Set `Host` header in curl |
| `context deadline exceeded` | Mixnet round-trip > timeout | Retry; mixnet delays can be long (up to 60s) |
| `PKI doc not available` | Dirauths not in consensus | Check `docker logs mix-dirauth-1` for voting errors |
| `ERROR: Layers 5 exceeds maximum` | Too many topology layers | Keep `[Debug] Layers ≤ 3` |
| `Document contains multiple entries` | Duplicate nodes in topology | Don't reuse mix node identifiers across layers |
| `GetService(proxy) failed` | Wrong service name in walletshield | Use `http_proxy` (the registered CBORPluginKaetzchen endpoint) |

## References

- Anon-RPC SPEC: https://github.com/privacy-ethereum/anon-rpc/blob/main/SPEC.md
- Anon-RPC demo: https://privacy-ethereum.github.io/anon-rpc/demo/
- Katzenpost: https://github.com/katzenpost/katzenpost
- KPS library: https://github.com/privacy-ethereum/kps
- KPS Go client: https://github.com/privacy-ethereum/kps/libs/go
