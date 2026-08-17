#!/usr/bin/env bash
# Interactive wizard for a fresh VPS: installs Docker if missing, generates
# or imports the validator wallet, creates secrets, and brings the pool up
# behind the bundled TLS reverse proxy.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_DIR"

NIMIQ_IMAGE="ghcr.io/nimiq/core-rs-albatross:latest"

step() { echo; echo "==> $1"; }

confirm() {
  local prompt="$1" reply
  read -rp "$prompt [y/N] " reply
  [[ "$reply" =~ ^[Yy]$ ]]
}

# Runs a binary baked into the official Nimiq image; no custom image needed.
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

step "Step 1/5: Docker"
if ! command -v docker >/dev/null 2>&1; then
  echo "Docker not found, installing..."
  curl -fsSL https://get.docker.com | sh
  systemctl enable --now docker
else
  echo "Docker already installed."
fi

step "Step 2/5: Secrets directory"
install -d -m 700 .secrets data/config

if [ ! -f .secrets/setup-token ]; then
  openssl rand -hex 32 > .secrets/setup-token
fi
if [ ! -f .secrets/session-secret ]; then
  openssl rand -hex 32 > .secrets/session-secret
fi

step "Step 3/5: Validator wallet"
if [ -f .secrets/validator-key ] && ! confirm "validator-key already exists, replace it?"; then
  echo "Keeping existing .secrets/validator-key"
elif confirm "Generate a new validator wallet using the official Nimiq image (recommended for a fresh validator)?"; then
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

  wallet_file=".secrets/wallet.json"
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

  # GoPool only ever needs the payout address's private key.
  install -m 600 /dev/stdin .secrets/validator-key <<<"$address_private_key"

  cat <<EOF

Generated a new validator identity. FULL key material (including the
validator/signing/voting keys needed to run the validator node itself,
not just GoPool) is saved to ${wallet_file} — back it up somewhere safe
offline, then consider deleting it from this server.

  validator_address: ${validator_address}

GoPool's payout signer (.secrets/validator-key) is set to this address's
private key.
EOF
else
  read -rsp "Paste the existing validator payout private key (hex): " key
  echo
  if [ -n "$key" ]; then
    install -m 600 /dev/stdin .secrets/validator-key <<<"$key"
  else
    echo "Skipped: create .secrets/validator-key (mode 600) before starting the daemon."
  fi
fi

step "Step 4/5: Domain / TLS"
if [ -f .env ] && grep -q GOPOOL_DOMAIN .env 2>/dev/null; then
  echo "GOPOOL_DOMAIN already set in .env: $(grep GOPOOL_DOMAIN .env | cut -d= -f2)"
else
  read -rp "Domain pointed at this server (DNS A record required, e.g. pool.example.com): " domain
  echo "GOPOOL_DOMAIN=${domain:-localhost}" >> .env
fi

step "Step 5/5: Start GoPool"
if confirm "Build and start the daemon + API + Caddy now?"; then
  docker compose -f deployments/docker-compose.yml up -d --build
  cat <<EOF

Done. Open https://$(grep GOPOOL_DOMAIN .env | cut -d= -f2) and enter this setup token:
$(cat .secrets/setup-token)

Caddy requests the TLS certificate on first request, so the first load may take a few seconds.

Check status any time with:
  docker compose -f deployments/docker-compose.yml logs -f
EOF
else
  echo "Skipped. Start later with: docker compose -f deployments/docker-compose.yml up -d --build"
fi
