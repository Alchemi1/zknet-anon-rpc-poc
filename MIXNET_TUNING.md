# Mixnet Tuning & Configuration Snapshot

## Optimization Goals

| Metric | Current | Target | Notes |
|--------|---------|--------|-------|
| HTTP proxy round-trip | ~63s | <10s | Poisson rate LambdaP=0.001 → 1/0.001=1000ms mean delay |
| PKI epoch duration | 20 min | configurable | Shorter epochs for faster key rotation testing |
| Descriptor upload window | 2m30s | - | Must catch this window after restart |
| Sphinx packet delay | 250-750ms per hop | - | Configurable via Debug section |

## Tunable Parameters

### In authority config (`[Parameters]` section)
- `Mu = 0.005` — Mixing strategy
- `LambdaP = 0.001` → increase to reduce Poisson scheduling delay
- `LambdaL = 0.0005` — Loop decoy rate
- `LambdaM = 0.2` — Mean rate
- `LambdaR = 0.0005` — Slack rate
- `LambdaGMaxDelay = 60000` — Max delay (ms)

### In node configs (`[Debug]` section)
- `UnwrapDelay = 250` — Sphinx unwrap delay (ms)
- `GatewayDelay = 500` — Gateway processing delay
- `ServiceDelay = 500` — Service node processing delay
- `KaetzchenDelay = 750` — Kaetzchen plugin delay
- `SchedulerSlack = 450` — Scheduler slack
- `SendSlack = 50` — Send slack
- `DecoySlack = 15000` — Decoy slack

## Configuration Snapshot (Standard)

A reproducible standard config includes:

1. **Authority configs** (`auth1-3/authority.toml`)
   - Consistent SphinxGeometry across all auths
   - Matching topology layers and NrHops
   - No duplicate node entries in topology
   - Persisted authority identity keys

2. **Mix/gateway/servicenode configs** (`mix1-3/`, `gateway1/`, `servicenode1/katzenpost.toml`)
   - WireKEM = "MLKEM768" for post-quantum transport
   - Consistent SphinxGeometry across all nodes
   - Correct PKI authority addresses
   - Persistent mix keys (patched)

3. **Client config** (`client/client.toml`)
   - Pinned gateway identity key
   - Matching SphinxGeometry
   - Correct voting authority list

4. **Key persistence** (this repo's patch)
   - Mix keys saved to `<dataDir>/mixkey-<epoch>.key`
   - Loaded on restart — no epoch wait needed
   - Survives container restarts

## Verification Checklist

- [ ] All 9 containers running
- [ ] PKI consensus achieved (3/3 signatures)
- [ ] http-proxy-server Kaetzchen plugin active on servicenode
- [ ] http-proxy-client connects to kpclientd
- [ ] eth_blockNumber query returns valid response through mixnet
- [ ] Mix keys persisted after restart (mixkey-*.key files exist)
- [ ] Descriptor upload completes within window after restart
