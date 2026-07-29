# Mixnet Tuning & Configuration

## Optimization Goals

| Metric | Current | Target | Notes |
|--------|---------|--------|-------|
| KPS RTT (eth_blockNumber) | ~1.2-3.2s | <2s | After tuning LambdaP=0.008 + reduced delays |
| PKI epoch duration | 20 min | configurable | Shorter epochs for faster key rotation testing |
| Descriptor upload window | 2m30s | - | Must catch this window after restart |

## Current Tuned Parameters (epoch 240868+)

### Authority configs (`auth1-3/authority.toml` `[Parameters]`)

| Parameter | Value | Description |
|-----------|-------|-------------|
| `Mu` | 0.005 | Mixing strategy |
| `LambdaP` | **0.008** | Poisson scheduling delay (~125ms mean) |
| `LambdaL` | 0.0005 | Loop decoy rate |
| `LambdaM` | 0.2 | Mean rate |
| `LambdaR` | 0.0005 | Slack rate |
| `LambdaGMaxDelay` | **30000ms** | Max gateway delay (was 60000ms) |

### Node configs (`mix1-3/gateway1/servicenode1` `[Debug]`)

| Parameter | Default | Tuned | Impact |
|-----------|---------|-------|--------|
| `UnwrapDelay` | 250ms | **50ms** | -200ms per hop × 5 = -1s |
| `GatewayDelay` | 500ms | **100ms** | -400ms gateway |
| `ServiceDelay` | 500ms | **100ms** | -400ms service node |
| `KaetzchenDelay` | 750ms | **200ms** | -550ms plugin |
| `SchedulerSlack` | 450ms | **100ms** | -350ms scheduling |
| `DecoySlack` | 15000ms | **5000ms** | Faster decoy timing |

**Estimated total improvement**: ~3-5s RTT → ~1-2s RTT (tuned)

## How tuning works

1. Edit the config files in `config/mixnet/`
2. Restart containers: `docker compose restart`
3. Wait for next PKI epoch (~20 min) for changes to take effect
4. Verify: `curl -s http://127.0.0.1:9206/stats` — check `last_rtt_ms`

## Configuration Snapshot

### Authority configs
- Consistent `SphinxGeometry` across all auths
- Matching topology layers and NrHops
- No duplicate node entries in topology
- Persisted authority identity keys

### Mix/gateway/servicenode configs
- WireKEM = "MLKEM768" for post-quantum transport
- Consistent `SphinxGeometry` across all nodes
- Correct PKI authority addresses
- Persistent mix keys (patched)

### Client config
- Pinned gateway identity key
- Matching `SphinxGeometry`
- Correct voting authority list

### WalletShield config
- Thin client config at `config/mixnet/client/thinclient.toml`
- Worker bundle at `config/mixnet/client/worker.js`
- KPS keys auto-generated at `walletshield/kps.key`
- `ProxyHTTPService = "http_proxy"` matching servicenode endpoint

## Verification

```bash
# Check all containers
docker ps --format "table {{.Names}}\t{{.Status}}"

# Check PKI consensus
curl -s http://127.0.0.1:3517/api/epoch | python3 -m json.tool

# Check KPS RTT
curl -s http://127.0.0.1:9206/stats | python3 -m json.tool

# Full demo
./demo.sh
```
