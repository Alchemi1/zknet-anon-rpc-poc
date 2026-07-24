#!/bin/bash

SERVICES_DIR="/home/alchemical1/src/ZKNet/zknet-anon-rpc-poc/config/mixnet"

# Fix log_dir in the Kaetzhen config
sed -i 's|log_dir = "/var/lib/katzenpost/servicenode1"|log_dir = "/var/lib/katzenpost/servicenode1"|' "$SERVICES_DIR/servicenode1/kaetzhen.tmp"

sed -i 's|log_level = "DEBUG"|log_level = "DEBUG"|' "$SERVICES_DIR/servicenode1/kaetzhen.tmp"

echo "Fixed log_dir in kaetzhen config:"
cat "$SERVICES_DIR/servicenode1/kaetzhen.tmp"
