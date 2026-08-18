#!/usr/bin/env bash
# Non-interactive validator wallet generator. Writes the full key set to
# $OUT_DIR/wallet.json and the payout private key to $OUT_DIR/validator-key.
# Used by Swarm deploys and as the non-wizard counterpart to vps-onboard.sh.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${OUT_DIR:-$REPO_DIR/.secrets}"
NIMIQ_IMAGE="${NIMIQ_IMAGE:-ghcr.io/nimiq/core-rs-albatross:latest}"
FORCE=0

while [ $# -gt 0 ]; do
  case "$1" in
    --force) FORCE=1 ;;
    --out-dir) OUT_DIR="$2"; shift ;;
    *) echo "Unknown argument: $1" >&2; exit 1 ;;
  esac
  shift
done

run_nimiq_tool() {
  docker run --rm --entrypoint "$1" "$NIMIQ_IMAGE"
}

extract_labeled_value() {
  local output="$1" label="$2" value
  value="$(sed -n "s/^${label}:[[:space:]]*//p" <<<"$output" | head -n1)"
  [ -n "$value" ] || { echo "Failed to parse '${label}' from nimiq-address output" >&2; exit 1; }
  printf '%s\n' "$value"
}

extract_bls_section_value() {
  local output="$1" section="$2" value
  value="$(awk -v header="# ${section}:" '
    $0 == header { found=1; next }
    found && NF { print; exit }
  ' <<<"$output")"
  [ -n "$value" ] || { echo "Failed to parse BLS section '${section}' from nimiq-bls output" >&2; exit 1; }
  printf '%s\n' "$value"
}

install -d -m 700 "$OUT_DIR"
wallet_file="$OUT_DIR/wallet.json"
key_file="$OUT_DIR/validator-key"

if [ -f "$wallet_file" ] && [ "$FORCE" -ne 1 ]; then
  echo "Refusing to overwrite $wallet_file (pass --force)." >&2
  exit 1
fi

echo "Pulling ${NIMIQ_IMAGE}..."
docker pull -q "$NIMIQ_IMAGE" >/dev/null

addr_out="$(run_nimiq_tool nimiq-address)"
signing_out="$(run_nimiq_tool nimiq-address)"
fee_out="$(run_nimiq_tool nimiq-address)"
bls_out="$(run_nimiq_tool nimiq-bls)"

validator_address="$(extract_labeled_value "$addr_out" "Address")"
address_private_key="$(extract_labeled_value "$addr_out" "Private Key")"
signing_private_key="$(extract_labeled_value "$signing_out" "Private Key")"
fee_private_key="$(extract_labeled_value "$fee_out" "Private Key")"
voting_public_key="$(extract_bls_section_value "$bls_out" "Public Key")"
voting_secret_key="$(extract_bls_section_value "$bls_out" "Secret Key")"

cat > "$wallet_file" <<JSON
{
  "validator_address": "$validator_address",
  "address_private_key": "$address_private_key",
  "signing_private_key": "$signing_private_key",
  "fee_private_key": "$fee_private_key",
  "voting_secret_key": "$voting_secret_key",
  "voting_public_key": "$voting_public_key"
}
JSON
chmod 600 "$wallet_file"
install -m 600 /dev/stdin "$key_file" <<<"$address_private_key"

cat <<EOF
Wrote full validator key material to ${wallet_file}
Wrote payout signer key to ${key_file}

  validator_address: ${validator_address}

Back up wallet.json offline. GoPool only reads validator-key (the payout
address private key). signing/fee/voting keys are for running the validator
node itself.
EOF
