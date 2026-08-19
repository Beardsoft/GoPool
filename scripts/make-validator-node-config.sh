#!/usr/bin/env bash
# Writes node-config.toml (the validator node's client.toml) from wallet.json.
# Contains the validator's signing/fee/voting keys: deploy it as a docker
# SECRET, never as a plain config.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${OUT_DIR:-$REPO_DIR/.secrets}"
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
[ -f "$wallet_file" ] || { echo "Missing $wallet_file (run generate-validator-wallet.sh first)" >&2; exit 1; }

out_file="$OUT_DIR/node-config.toml"
python3 - "$wallet_file" "$NETWORK" "$out_file" <<'PY'
import json, sys
wallet, network, out = sys.argv[1], sys.argv[2], sys.argv[3]
w = json.load(open(wallet))

# Seed nodes per network (from client.example.toml). Without these the node
# has nothing to bootstrap the DHT from and stays at 0 peers.
SEEDS = {
    "test-albatross": [
        "/dns4/seed1.pos.nimiq-testnet.com/tcp/8443/wss",
        "/dns4/seed2.pos.nimiq-testnet.com/tcp/8443/wss",
        "/dns4/seed3.pos.nimiq-testnet.com/tcp/8443/wss",
        "/dns4/seed4.pos.nimiq-testnet.com/tcp/8443/wss",
    ],
    "main-albatross": [
        "/dns4/aurora.seed.nimiq.com/tcp/443/wss",
        "/dns4/catalyst.seed.nimiq.network/tcp/443/wss",
        "/dns4/cipher.seed.nimiq-network.com/tcp/443/wss",
        "/dns4/eclipse.seed.nimiq.cloud/tcp/443/wss",
        "/dns4/lumina.seed.nimiq.systems/tcp/443/wss",
        "/dns4/nebula.seed.nimiq.com/tcp/443/wss",
        "/dns4/nexus.seed.nimiq.network/tcp/443/wss",
        "/dns4/polaris.seed.nimiq-network.com/tcp/443/wss",
        "/dns4/photon.seed.nimiq.cloud/tcp/443/wss",
        "/dns4/pulsar.seed.nimiq.systems/tcp/443/wss",
        "/dns4/quasar.seed.nimiq.com/tcp/443/wss",
        "/dns4/solstice.seed.nimiq.network/tcp/443/wss",
        "/dns4/vortex.seed.nimiq.cloud/tcp/443/wss",
        "/dns4/zenith.seed.nimiq.systems/tcp/443/wss",
    ],
}
seeds = SEEDS.get(network)
if not seeds:
    sys.exit("No seed nodes known for network '%s' (add to SEEDS in this script)" % network)

lines = [
    "[network]",
    # 2.0.0-pre defaults this to 0, which panics the validator's request
    # handlers (mpsc bounded channel requires buffer > 0). Must be > 0.
    "network_buffer_size = 1024",
    "seed_nodes = [",
    *[ '  { address = "%s" },' % s for s in seeds ],
    "]",
    "",
    "[consensus]",
    'network = "%s"' % network,
    "",
    "[validator]",
    'validator_address = "%s"' % w["validator_address"],
    'signing_key = "%s"' % w["signing_private_key"],
    'fee_key = "%s"' % w["fee_private_key"],
    'voting_key = "%s"' % w["voting_secret_key"],
    "automatic_reactivate = true",
    "",
    # JSON-RPC API: required by the validator activator (importRawKey,
    # unlockAccount, sendNewValidatorTransaction). Bind 0.0.0.0 so tooling in
    # the same docker network can reach it; never publish the port.
    "[rpc-server]",
    'bind = "0.0.0.0"',
    "port = 8648",
    "",
]
open(out, "w").write("\n".join(lines))
PY
# 644 so Compose bind-mounts are readable by the validator user (nimiq).
# The parent .secrets directory stays mode 700.
chmod 644 "$out_file"

cat <<EOF
Wrote $out_file for network ${NETWORK}

Deploy as a secret, e.g.:
  docker secret create gopool_test_node_config ${out_file}
EOF
