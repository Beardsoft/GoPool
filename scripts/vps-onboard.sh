#!/usr/bin/env bash
# Interactive or one-shot wizard for a git checkout. VPS installs should use
# scripts/install.sh (curl|bash, no clone). This path writes secrets in the
# repo and starts deployments/docker-compose.yml (supports --build).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=install.sh
source "$SCRIPT_DIR/install.sh"

if [[ "${BASH_SOURCE[0]}" != "$0" ]]; then
  return 0 2>/dev/null || exit 0
fi

REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_DIR"

ASSUME_YES=0
GENERATE_WALLET=0
NO_WALLET=0
DOMAIN=""
NETWORK="main-albatross"
RPC_URL=""
SKIP_START=0
BUILD_IMAGES=0
TLS_MODE=""

usage() {
  cat <<EOF
Usage: sudo ./scripts/vps-onboard.sh [options]

For a production VPS, prefer curl|bash of scripts/install.sh (no git clone).
This wizard is for an existing checkout.

  --yes                 non-interactive (requires --domain)
  --domain NAME         DNS A record pointing at this server
  --network NAME        main-albatross (default) or test-albatross
  --rpc-url URL         pool RPC endpoint (defaults from --network)
  --tls auto|proxy      auto: Caddy + Let's Encrypt; proxy: HTTP behind TLS proxy
  --generate-wallet     create a new validator identity
  --no-wallet           require an existing .secrets/validator-key
  --skip-start          write secrets/hints but do not start Compose
  --build               build images locally instead of pulling GHCR
  -h, --help            show this help
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --yes|-y) ASSUME_YES=1 ;;
    --domain) DOMAIN="$2"; shift ;;
    --network) NETWORK="$2"; shift ;;
    --rpc-url) RPC_URL="$2"; shift ;;
    --tls) TLS_MODE="$2"; shift ;;
    --generate-wallet) GENERATE_WALLET=1 ;;
    --no-wallet) NO_WALLET=1 ;;
    --skip-start) SKIP_START=1 ;;
    --build) BUILD_IMAGES=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage >&2; exit 1 ;;
  esac
  shift
done

if [ -z "$RPC_URL" ]; then
  RPC_URL="$(gopool_rpc_url_for_network "$NETWORK")"
fi
if [ -n "$TLS_MODE" ] && [ "$TLS_MODE" != auto ] && [ "$TLS_MODE" != proxy ]; then
  echo "--tls must be auto or proxy" >&2
  exit 1
fi
if [ "$ASSUME_YES" = 1 ] && [ -z "$DOMAIN" ]; then
  echo "--domain is required with --yes" >&2
  exit 1
fi
if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

step() { echo; echo "==> $1"; }

confirm() {
  local prompt="$1" reply
  read -rp "$prompt [y/N] " reply
  [[ "$reply" =~ ^[Yy]$ ]]
}

step "Step 1/5: Packages and Docker"
pkgs=()
command -v curl >/dev/null || pkgs+=(curl ca-certificates)
command -v openssl >/dev/null || pkgs+=(openssl)
command -v python3 >/dev/null || pkgs+=(python3)
if [ ${#pkgs[@]} -gt 0 ]; then
  apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "${pkgs[@]}"
fi
if ! command -v docker >/dev/null 2>&1; then
  echo "Docker not found, installing..."
  curl -fsSL https://get.docker.com | sh
  systemctl enable --now docker
else
  echo "Docker already installed."
fi
if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose plugin is required." >&2
  exit 1
fi
if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q '^Status: active'; then
  ufw allow 80/tcp >/dev/null
  ufw allow 443/tcp >/dev/null
  echo "Opened ports 80 and 443 in ufw."
fi

step "Step 2/5: Secrets directory"
install -d -m 700 .secrets data/config

if [ ! -f .secrets/setup-token ]; then
  openssl rand -hex 32 > .secrets/setup-token
fi
if [ ! -f .secrets/session-secret ]; then
  openssl rand -hex 32 > .secrets/session-secret
fi
chmod 600 .secrets/setup-token .secrets/session-secret

step "Step 3/5: Validator wallet"
if [ -f .secrets/validator-key ]; then
  if [ "$ASSUME_YES" = 1 ]; then
    echo "Keeping existing .secrets/validator-key"
  elif confirm "validator-key already exists, replace it?"; then
    GENERATE_WALLET=1
    rm -f .secrets/validator-key
  else
    echo "Keeping existing .secrets/validator-key"
  fi
fi

if [ ! -f .secrets/validator-key ]; then
  if [ "$NO_WALLET" = 1 ]; then
    echo "Missing .secrets/validator-key (refusing to generate because --no-wallet)." >&2
    exit 1
  elif [ "$ASSUME_YES" = 1 ] || [ "$GENERATE_WALLET" = 1 ]; then
    GENERATE_WALLET=1
  elif confirm "Generate a new validator wallet using the official Nimiq image (recommended for a fresh validator)?"; then
    GENERATE_WALLET=1
  fi
fi

if [ "$GENERATE_WALLET" = 1 ] && [ ! -f .secrets/validator-key ]; then
  gopool_generate_wallet .secrets
elif [ ! -f .secrets/validator-key ]; then
  read -rsp "Paste the existing validator payout private key (hex): " key
  echo
  if [ -n "$key" ]; then
    install -m 600 /dev/stdin .secrets/validator-key <<<"$key"
    cat <<EOF

Note: a pasted payout key lets GoPool pay out, but the validator node
cannot run without the full key set. To add it later, save the full key
set to .secrets/wallet.json and re-run this wizard.
EOF
  else
    echo "Skipped: create .secrets/validator-key (mode 600) before starting the daemon."
  fi
fi

if [ -f .secrets/wallet.json ] && [ ! -f .secrets/node-config.toml ]; then
  if [ "$ASSUME_YES" = 1 ]; then
    validator_network="$NETWORK"
  else
    read -rp "Network for the validator node [${NETWORK}]: " validator_network
    validator_network="${validator_network:-$NETWORK}"
  fi
  NETWORK="$validator_network"
  RPC_URL="$(gopool_rpc_url_for_network "$NETWORK")"
  gopool_write_node_config .secrets/wallet.json "$NETWORK" .secrets/node-config.toml
fi

step "Step 4/5: Domain / TLS"
if [ -z "$DOMAIN" ]; then
  if [ -f .env ] && grep -q '^GOPOOL_DOMAIN=' .env; then
    DOMAIN="$(grep '^GOPOOL_DOMAIN=' .env | cut -d= -f2-)"
    echo "GOPOOL_DOMAIN already set in .env: $DOMAIN"
  else
    read -rp "Domain pointed at this server (DNS A record required, e.g. pool.example.com): " DOMAIN
    DOMAIN="${DOMAIN:-localhost}"
  fi
fi
TLS_MODE="$(gopool_choose_tls_mode "$DOMAIN" "$TLS_MODE")"
SITE="$(gopool_site_address "$DOMAIN" "$TLS_MODE")"
RESOLVED_IP="$(gopool_resolve_ipv4 "$DOMAIN")"
PRIMARY_IP="$(gopool_primary_ipv4)"
gopool_set_env_var .env GOPOOL_DOMAIN "$DOMAIN"
gopool_set_env_var .env GOPOOL_SITE "$SITE"
gopool_set_env_var .env GOPOOL_IMAGE "${GOPOOL_IMAGE:-ghcr.io/beardsoft/gopool:latest}"
if [ "$TLS_MODE" = proxy ]; then
  echo "Domain ${DOMAIN} does not point at this server (${PRIMARY_IP:-unknown}; DNS=${RESOLVED_IP:-unresolved})."
  echo "Serving HTTP on :80. Point your reverse proxy at http://${PRIMARY_IP:-THIS_IP}:80"
else
  echo "Caddy will request a Let's Encrypt certificate for ${DOMAIN}."
  echo "Open ports 80 and 443 on any cloud/security-group firewall in front of this VPS."
fi

VALIDATOR_ADDRESS=""
if [ -f .secrets/wallet.json ]; then
  VALIDATOR_ADDRESS="$(gopool_wallet_address .secrets/wallet.json)"
fi
gopool_write_setup_hints data/config/setup-hints.json "$VALIDATOR_ADDRESS" "$NETWORK" "$RPC_URL"

step "Step 5/5: Start GoPool"
start_stack() {
  local compose=(docker compose --env-file .env -f deployments/docker-compose.yml)
  local services=(gopool gopool-api caddy)
  if [ -f .secrets/node-config.toml ]; then
    services+=(gopool-validator)
  else
    echo "No node-config.toml: starting without the gopool-validator service."
  fi
  if [ "$BUILD_IMAGES" = 1 ]; then
    "${compose[@]}" up -d --build "${services[@]}"
    return
  fi
  "${compose[@]}" pull "${services[@]}"
  "${compose[@]}" up -d "${services[@]}"
}

if [ "$SKIP_START" = 1 ]; then
  echo "Skipped start (--skip-start)."
elif [ "$ASSUME_YES" = 1 ] || confirm "Pull images and start the daemon + API + Caddy now?"; then
  start_stack
else
  echo "Skipped. Start later with: docker compose --env-file .env -f deployments/docker-compose.yml up -d"
fi

SETUP_TOKEN="$(cat .secrets/setup-token)"
SETUP_URL="$(gopool_setup_url "$DOMAIN" "$SETUP_TOKEN")"
if [ "$TLS_MODE" = proxy ]; then
  TLS_NOTE="Your reverse proxy must forward https://${DOMAIN} to http://${PRIMARY_IP:-THIS_IP}:80
(scheme HTTP, forward port 80). Then open the setup URL."
else
  TLS_NOTE="Caddy requests the TLS certificate on first request; the first load may take a few seconds."
fi
cat <<EOF

Done. Open this URL to finish setup (token is already in the link):

  ${SETUP_URL}

The wizard is pre-filled from the generated wallet. Confirm the pool name,
fee, and operators, then launch. No Compose restart after that.

${TLS_NOTE}

Still manual:
  1. DNS A record, or a reverse proxy to http://${PRIMARY_IP:-THIS_IP}:80
  2. Backup .secrets/wallet.json offline (keep it until the daemon registers)
  3. Mainnet: send at least 101000 NIM to the validator address. Testnet: the daemon faucets, registers, and self-stakes.

Check status any time with:
  docker compose --env-file .env -f deployments/docker-compose.yml logs -f
EOF
