#!/usr/bin/env python3
import os
import re

SERVICES_DIR = "/home/alchemical1/src/ZKNet/zknet-anon-rpc-poc/config/mixnet"

# Read the kaetzhen file
with open(f"{SERVICES_DIR}/servicenode1/kaetzchen.tmp", "r") as f:
    content = f.read()

# Fix the config - ensure it uses http-proxy-server
# Fix command first
content = content.replace(
    'Command = "/usr/local/bin/proxy-kaetzhen"',
    'Command = "/usr/local/bin/http-proxy-server"'
)

# Ensure capability is http_proxy
content = content.replace(
    'Capability = "proxy"',
    'Capability = "http_proxy"'
)

# Ensure endpoint is http_proxy
content = content.replace(
    'Endpoint = "proxy"',
    'Endpoint = "http_proxy"'
)

# Write back
with open(f"{SERVICES_DIR}/servicenode1/kaetzchen.tmp", "w") as f:
    f.write(content)

print("Updated kaetzhen config successfully:")
print("=" * 50)
print(content)
print("=" * 50)
