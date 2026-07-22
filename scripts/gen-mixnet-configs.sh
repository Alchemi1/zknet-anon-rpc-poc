#!/bin/bash
set -euo pipefail

# gen-mixnet-configs.sh
# Generates Katzenpost mixnet PKI + node configs for local dev
# Topology: 3 dirauths, 3 mixes, 1 gateway, 1 servicenode, 1 client

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG_DIR="$PROJECT_ROOT/config/mixnet"
PKIDATA_DIR="$CONFIG_DIR/pkidata"

mkdir -p "$PKIDATA_DIR"

for i in 1 2 3; do
    mkdir -p "$CONFIG_DIR/auth$i"
    mkdir -p "$CONFIG_DIR/mix$i"
done
mkdir -p "$CONFIG_DIR/gateway1"
mkdir -p "$CONFIG_DIR/servicenode1"
mkdir -p "$CONFIG_DIR/client"

# SphinxGeometry (shared across all nodes)
SPHINX_GEOMETRY='[SphinxGeometry]
NIKEName = "x25519"
NrHops = 5
HeaderLength = 476
RoutingInfoLength = 410
PerHopRoutingInfoLength = 82
PacketLength = 3082
SURBLength = 572
ForwardPayloadLength = 2574
UserForwardPayloadLength = 2000
NextNodeHopLength = 65
SPRPKeyMaterialLength = 48
SphinxPlaintextHeaderLength = 2
PayloadTagLength = 32'

echo "Generating mixnet configuration..."

# Generate dirauth keypairs
for i in 1 2 3; do
    AUTH_DIR="$CONFIG_DIR/auth$i"
    if [ ! -f "$AUTH_DIR/identity.key" ]; then
        echo "  Generating keypair for dirauth-$i..."
        openssl genpkey -algorithm ed25519 -out "$AUTH_DIR/identity.key" 2>/dev/null || echo "placeholder-key" > "$AUTH_DIR/identity.key"
        openssl pkey -in "$AUTH_DIR/identity.key" -pubout -out "$AUTH_DIR/identity.pub" 2>/dev/null || echo "placeholder-pub" > "$AUTH_DIR/identity.pub"
    fi
done

# Consortium YAML
cat > "$PKIDATA_DIR/consortium.yaml" << EOF
version: 1
consensus_type: threshold
consensus_epsilon: 0.2
voting_interval: 1m
cert_expiry: 168h
authorities:
  - identifier: auth1
    identity_key_file: /var/lib/katzenpost/auth1/identity.pub
    address: 127.0.0.1:12349
  - identifier: auth2
    identity_key_file: /var/lib/katzenpost/auth2/identity.pub
    address: 127.0.0.1:12449
  - identifier: auth3
    identity_key_file: /var/lib/katzenpost/auth3/identity.pub
    address: 127.0.0.1:12549
EOF

# Dirauth configs
for i in 1 2 3; do
    AUTH_DIR="$CONFIG_DIR/auth$i"
    PORT=$((12348 + i))
    cat > "$AUTH_DIR/authority.toml" << EOF
[Server]
Identifier = "auth${i}"
DataDir = "/var/lib/katzenpost"
LogLevel = "INFO"

[Server.Listen]
  [Server.Listen.Tcp]
    Address = "127.0.0.1:${PORT}"
    Network = "tcp"

$SPHINX_GEOMETRY

[PKI]
ConsensusConfig = "/var/lib/katzenpost/pkidata/consortium.yaml"
EOF
done

# Mix node configs
for i in 1 2 3; do
    MIX_DIR="$CONFIG_DIR/mix$i"
    PORT=$((12450 + i * 10))
    cat > "$MIX_DIR/katzenpost.toml" << EOF
[Server]
Identifier = "mix${i}"
DataDir = "/var/lib/katzenpost"
LogLevel = "INFO"

[Server.Listen]
  [Server.Listen.Tcp]
    Address = "127.0.0.1:${PORT}"
    Network = "tcp"

$SPHINX_GEOMETRY

[PKI]
  [PKI.DirectoryAuthority]
    Addresses = ["127.0.0.1:12349", "127.0.0.1:12449", "127.0.0.1:12549"]
EOF
done

# Gateway config
GATEWAY_DIR="$CONFIG_DIR/gateway1"
cat > "$GATEWAY_DIR/katzenpost.toml" << EOF
[Server]
Identifier = "gateway1"
DataDir = "/var/lib/katzenpost"
LogLevel = "INFO"

[Server.Listen]
  [Server.Listen.Tcp]
    Address = "127.0.0.1:12601"
    Network = "tcp"

$SPHINX_GEOMETRY

[PKI]
  [PKI.DirectoryAuthority]
    Addresses = ["127.0.0.1:12349", "127.0.0.1:12449", "127.0.0.1:12549"]
EOF

# Servicenode config with http-proxy Kaetzchen
SVC_DIR="$CONFIG_DIR/servicenode1"
cat > "$SVC_DIR/katzenpost.toml" << EOF
[Server]
Identifier = "servicenode1"
DataDir = "/var/lib/katzenpost"
LogLevel = "INFO"

[Server.Listen]
  [Server.Listen.Tcp]
    Address = "127.0.0.1:12602"
    Network = "tcp"

$SPHINX_GEOMETRY

[PKI]
  [PKI.DirectoryAuthority]
    Addresses = ["127.0.0.1:12349", "127.0.0.1:12449", "127.0.0.1:12549"]

[Services]
  [[Services]]
    Name = "proxy"
    Kaetzchen = "http-proxy"
    Capabilities = ["http-proxy"]
    ActivateCmdLine = "echo http-proxy running"
    DeactivateCmdLine = "echo http-proxy stopped"
EOF

echo "Mixnet configuration generated in $CONFIG_DIR"
