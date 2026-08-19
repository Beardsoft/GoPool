#!/usr/bin/env bash
# Writes node-config.toml (the validator node's client.toml) from wallet.json.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=install.sh
source "$SCRIPT_DIR/install.sh"

OUT_DIR="${OUT_DIR:-$(cd "$SCRIPT_DIR/.." && pwd)/.secrets}"
NETWORK="${NETWORK:-test-albatross}"

while [ $# -gt 0 ]; do
  case "$1" in
    --network) NETWORK="$2"; shift ;;
    --out-dir) OUT_DIR="$2"; shift ;;
    *) echo "Unknown argument: $1" >&2; exit 1 ;;
  esac
  shift
done

wallet_file="$OUT_DIR/wallet.json"
out_file="$OUT_DIR/node-config.toml"
gopool_write_node_config "$wallet_file" "$NETWORK" "$out_file"
cat <<EOF
Wrote $out_file for network ${NETWORK}

Deploy as a secret, e.g.:
  docker secret create gopool_test_node_config ${out_file}
EOF
