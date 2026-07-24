#!/usr/bin/env python3
import re
import os

SERVICES_DIR = "/home/alchemical1/src/ZKNet/zknet-anon-rpc-poc/config/mixnet"

# Read the kaetzhen file
with open(f"{SERVICES_DIR}/servicenode1/kaetzchen.tmp", "r") as f:
    content = f.read()

# Fix the command - change proxy-kaetzhen to http-proxy-server
content = content.replace("Command = \"/usr/local/bin/proxy-kaetzhen\"", "Command = \"/usr/local/bin/http-proxy-server\"")

# Fix the capability 
content = content.replace('Capability = "proxy"', 'Capability = "http_proxy"')

# Fix the endpoint
content = content.replace('Endpoint = "proxy"', 'Endpoint = "http_proxy"')

# Write back
with open(f"{SERVICES_DIR}/servicenode1/kaetzchen.tmp", "w") as f:
    f.write(content)

print("Fixed kaetzhen config:")
print(content)
