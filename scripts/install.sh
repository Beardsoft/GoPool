#!/usr/bin/env bash
# One-shot VPS installer. Does not check out source — pulls published images and
# writes compose + secrets locally:
#   curl -fsSL https://raw.githubusercontent.com/Beardsoft/GoPool/master/scripts/install.sh \
#     | sudo bash -s -- --domain pool.example.com
set -euo pipefail

GOPOOL_DEFAULT_IMAGE="${GOPOOL_IMAGE:-ghcr.io/beardsoft/gopool:latest}"
NIMIQ_TOOL_IMAGE="${NIMIQ_IMAGE:-ghcr.io/nimiq/core-rs-albatross:latest}"
NIMIQ_NODE_IMAGE="${NIMIQ_NODE_IMAGE:-ghcr.io/nimiq/core-rs-albatross:2.0.0-pre}"

gopool_rpc_url_for_network() {
  case "$1" in
    test-albatross) printf '%s\n' 'https://rpc-testnet.nimiqscan.com' ;;
    *) printf '%s\n' 'https://rpc-mainnet.nimiqscan.com' ;;
  esac
}

gopool_setup_url() {
  local domain="$1" token="$2"
  printf 'https://%s/setup?token=%s\n' "$domain" "$token"
}

gopool_write_setup_hints() {
  local dest="$1" address="$2" network="$3" rpc="$4" fee="${5:-$2}"
  cat > "$dest" <<JSON
{
  "validator_address": "$address",
  "pool_fee_wallet": "$fee",
  "network": "$network",
  "rpc_url": "$rpc"
}
JSON
}

gopool_wallet_address() {
  python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("validator_address",""))' "$1"
}

gopool_set_env_var() {
  local file="$1" key="$2" value="$3"
  if [ -f "$file" ]; then
    grep -v "^${key}=" "$file" > "${file}.tmp" || true
    mv "${file}.tmp" "$file"
  fi
  echo "${key}=${value}" >> "$file"
}

gopool_local_ipv4s() {
  ip -4 -o addr show scope global 2>/dev/null | awk '{print $4}' | cut -d/ -f1
}

gopool_resolve_ipv4() {
  getent ahostsv4 "$1" 2>/dev/null | awk '{print $1; exit}' || true
}

gopool_primary_ipv4() {
  local ips
  ips="$(gopool_local_ipv4s || true)"
  printf '%s\n' "${ips%%$'\n'*}"
}

gopool_domain_points_here() {
  local resolved="$1" ip
  [ -n "$resolved" ] || return 1
  while read -r ip; do
    [ -n "$ip" ] || continue
    [ "$ip" = "$resolved" ] && return 0
  done <<<"$(gopool_local_ipv4s)"
  return 1
}

gopool_choose_tls_mode() {
  local domain="$1" override="${2:-}" resolved
  case "$override" in
    auto|proxy) printf '%s\n' "$override"; return ;;
    '') ;;
    *) echo "Unknown --tls mode: $override (use auto or proxy)" >&2; return 1 ;;
  esac
  if [ "$domain" = "localhost" ] || [ "$domain" = "127.0.0.1" ]; then
    printf '%s\n' auto
    return
  fi
  resolved="$(gopool_resolve_ipv4 "$domain")"
  if gopool_domain_points_here "$resolved"; then
    printf '%s\n' auto
  else
    printf '%s\n' proxy
  fi
}

gopool_site_address() {
  local domain="$1" mode="$2"
  if [ "$mode" = proxy ]; then
    printf 'http://%s\n' "$domain"
  else
    printf '%s\n' "$domain"
  fi
}

gopool_write_caddyfile() {
  cat > "$1" <<'EOF'
{$GOPOOL_SITE} {
	reverse_proxy gopool-api:8080
}
EOF
}

gopool_write_compose() {
  local dest="$1" node_image="$2"
  cat > "$dest" <<EOF
name: gopool
services:
  gopool:
    image: \${GOPOOL_IMAGE:-ghcr.io/beardsoft/gopool:latest}
    container_name: gopool
    ports: ["127.0.0.1:9100:9100"]
    volumes:
      - ./data:/root/data
      - ./data/config:/root/config:ro
    environment:
      SERVICE: daemon
      SQLITE_DB: /root/data/pool.db
      CONFIG_FILE: /root/config/config.json
      POOL_PRIVATE_KEY_FILE: /run/secrets/gopool_validator_key
      POOL_WALLET_JSON_FILE: /run/secrets/gopool_wallet_json
      VALIDATOR_RPC_URL: http://gopool-validator:8648
      FAUCET_URL: https://faucet.pos.nimiq-testnet.com/tapit
    secrets: [gopool_validator_key, gopool_wallet_json]
    healthcheck:
      test: ["CMD-SHELL", "test -r /run/secrets/gopool_validator_key"]
      interval: 30s
      timeout: 5s
      retries: 3
    restart: unless-stopped

  gopool-api:
    image: \${GOPOOL_IMAGE:-ghcr.io/beardsoft/gopool:latest}
    container_name: gopool-api
    ports: ["127.0.0.1:52412:8080"]
    volumes:
      - ./data:/root/data
      - ./data/config:/root/config:rw
    environment:
      SERVICE: api
      SQLITE_DB: /root/data/pool.db
      CONFIG_FILE: /root/config/config.json
      POOL_API_ADDR: :8080
      POOL_SETUP_TOKEN_FILE: /run/secrets/gopool_setup_token
      POOL_SESSION_SECRET_FILE: /run/secrets/gopool_session_secret
    secrets: [gopool_setup_token, gopool_session_secret]
    healthcheck:
      test: ["CMD-SHELL", "test -r /run/secrets/gopool_setup_token && test -r /run/secrets/gopool_session_secret"]
      interval: 30s
      timeout: 5s
      retries: 3
    depends_on: [gopool]
    restart: unless-stopped

  gopool-validator:
    image: ${node_image}
    container_name: gopool-validator
    volumes:
      - validator_data:/home/nimiq/.nimiq
    secrets:
      - source: gopool_node_config
        target: /home/nimiq/.nimiq/client.toml
    restart: unless-stopped

  caddy:
    image: caddy:2-alpine
    container_name: gopool-caddy
    ports: ["80:80", "443:443"]
    environment:
      GOPOOL_DOMAIN: \${GOPOOL_DOMAIN:-localhost}
      GOPOOL_SITE: \${GOPOOL_SITE:-\${GOPOOL_DOMAIN:-localhost}}
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
    depends_on: [gopool-api]
    restart: unless-stopped

volumes:
  caddy_data:
  validator_data:

secrets:
  gopool_validator_key:
    file: ./.secrets/validator-key
  gopool_wallet_json:
    file: ./.secrets/wallet.json
  gopool_setup_token:
    file: ./.secrets/setup-token
  gopool_session_secret:
    file: ./.secrets/session-secret
  gopool_node_config:
    file: ./.secrets/node-config.toml
EOF
}

gopool_extract_labeled_value() {
  local output="$1" label="$2" value
  value="$(sed -n "s/^${label}:[[:space:]]*//p" <<<"$output" | head -n1)"
  [ -n "$value" ] || { echo "Failed to parse '${label}' from nimiq-address output" >&2; return 1; }
  printf '%s\n' "$value"
}

gopool_extract_bls_section_value() {
  local output="$1" section="$2" value
  value="$(awk -v header="# ${section}:" '
    $0 == header { found=1; next }
    found && NF { print; exit }
  ' <<<"$output")"
  [ -n "$value" ] || { echo "Failed to parse BLS section '${section}' from nimiq-bls output" >&2; return 1; }
  printf '%s\n' "$value"
}

gopool_generate_wallet() {
  local out_dir="$1" image="${2:-$NIMIQ_TOOL_IMAGE}"
  local wallet_file key_file addr_out signing_out fee_out bls_out
  local validator_address address_private_key signing_private_key fee_private_key
  local voting_public_key voting_secret_key
  install -d -m 700 "$out_dir"
  wallet_file="$out_dir/wallet.json"
  key_file="$out_dir/validator-key"
  echo "Pulling ${image}..."
  docker pull -q "$image" >/dev/null
  addr_out="$(docker run --rm --entrypoint nimiq-address "$image")"
  signing_out="$(docker run --rm --entrypoint nimiq-address "$image")"
  fee_out="$(docker run --rm --entrypoint nimiq-address "$image")"
  bls_out="$(docker run --rm --entrypoint nimiq-bls "$image")"
  validator_address="$(gopool_extract_labeled_value "$addr_out" "Address")"
  address_private_key="$(gopool_extract_labeled_value "$addr_out" "Private Key")"
  signing_private_key="$(gopool_extract_labeled_value "$signing_out" "Private Key")"
  fee_private_key="$(gopool_extract_labeled_value "$fee_out" "Private Key")"
  voting_public_key="$(gopool_extract_bls_section_value "$bls_out" "Public Key")"
  voting_secret_key="$(gopool_extract_bls_section_value "$bls_out" "Secret Key")"
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
  echo "Wrote full validator key material to ${wallet_file}"
  echo "  validator_address: ${validator_address}"
}

gopool_write_node_config() {
  local wallet_file="$1" network="$2" out_file="$3"
  [ -f "$wallet_file" ] || { echo "Missing $wallet_file" >&2; return 1; }
  python3 - "$wallet_file" "$network" "$out_file" <<'PY'
import json, sys
wallet, network, out = sys.argv[1], sys.argv[2], sys.argv[3]
w = json.load(open(wallet))
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
    sys.exit("No seed nodes known for network '%s'" % network)
lines = [
    "[network]",
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
    "[rpc-server]",
    'bind = "0.0.0.0"',
    "port = 8648",
    "",
]
open(out, "w").write("\n".join(lines))
PY
  chmod 644 "$out_file"
}

# Sourced by tests / vps-onboard.sh. curl|bash has no BASH_SOURCE — run main.
if [[ -n "${BASH_SOURCE[0]:-}" && "${BASH_SOURCE[0]}" != "$0" ]]; then
  return 0 2>/dev/null || exit 0
fi

INSTALL_DIR="${GOPOOL_DIR:-/opt/gopool}"
DOMAIN=""
NETWORK="main-albatross"
RPC_URL=""
GENERATE_WALLET=1
TLS_MODE=""
SKIP_START=0

usage() {
  cat <<EOF
Install GoPool on a fresh Ubuntu/Debian VPS. Pulls published images; does not
clone the source repository.

  curl -fsSL https://raw.githubusercontent.com/Beardsoft/GoPool/master/scripts/install.sh \\
    | sudo bash -s -- --domain pool.example.com

Writes /opt/gopool/{compose.yml,Caddyfile,.env,.secrets,data}, generates a
validator wallet, and starts the stack. Point DNS at the server (ports 80/443)
or put a reverse proxy in front.

Options:
  --domain NAME     DNS name for the pool (required)
  --network NAME    main-albatross (default) or test-albatross
  --rpc-url URL     pool RPC endpoint (defaults from --network)
  --tls auto|proxy  auto: Caddy + Let's Encrypt; proxy: HTTP behind TLS proxy
  --dir PATH        install directory (default /opt/gopool)
  --no-wallet       do not generate a wallet (you must add .secrets/validator-key)
  --skip-start      write files but do not start Compose
  -h, --help        show this help
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --domain) DOMAIN="$2"; shift ;;
    --network) NETWORK="$2"; shift ;;
    --rpc-url) RPC_URL="$2"; shift ;;
    --tls) TLS_MODE="$2"; shift ;;
    --dir) INSTALL_DIR="$2"; shift ;;
    --no-wallet) GENERATE_WALLET=0 ;;
    --skip-start) SKIP_START=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage >&2; exit 1 ;;
  esac
  shift
done

if [ -z "$RPC_URL" ]; then
  RPC_URL="$(gopool_rpc_url_for_network "$NETWORK")"
fi
if [ -z "$DOMAIN" ]; then
  echo "Missing --domain. Example:" >&2
  echo "  curl -fsSL https://raw.githubusercontent.com/Beardsoft/GoPool/master/scripts/install.sh | sudo bash -s -- --domain pool.example.com" >&2
  exit 1
fi
if [ -n "$TLS_MODE" ] && [ "$TLS_MODE" != auto ] && [ "$TLS_MODE" != proxy ]; then
  echo "--tls must be auto or proxy" >&2
  exit 1
fi
if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root (sudo)." >&2
  exit 1
fi

step() { echo; echo "==> $1"; }

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

install -d -m 755 "$INSTALL_DIR"
cd "$INSTALL_DIR"

if [ -d .git ]; then
  echo "Existing source checkout in ${INSTALL_DIR}; switching to image-only layout."
fi
if [ -f deployments/docker-compose.yml ]; then
  echo "Stopping previous source-tree Compose stack..."
  docker compose --env-file .env -f deployments/docker-compose.yml down >/dev/null 2>&1 || true
fi

step "Step 2/5: Secrets and stack files"
install -d -m 700 .secrets data/config
gopool_write_caddyfile Caddyfile
gopool_write_compose compose.yml "$NIMIQ_NODE_IMAGE"

if [ ! -f .secrets/setup-token ]; then
  openssl rand -hex 32 > .secrets/setup-token
fi
if [ ! -f .secrets/session-secret ]; then
  openssl rand -hex 32 > .secrets/session-secret
fi
chmod 600 .secrets/setup-token .secrets/session-secret

step "Step 3/5: Validator wallet"
if [ ! -f .secrets/validator-key ]; then
  if [ "$GENERATE_WALLET" = 0 ]; then
    echo "Missing .secrets/validator-key (refusing to generate because --no-wallet)." >&2
    exit 1
  fi
  gopool_generate_wallet .secrets
fi
if [ -f .secrets/wallet.json ] && [ ! -f .secrets/node-config.toml ]; then
  gopool_write_node_config .secrets/wallet.json "$NETWORK" .secrets/node-config.toml
  echo "Wrote .secrets/node-config.toml for network ${NETWORK}"
fi

step "Step 4/5: Domain / TLS"
TLS_MODE="$(gopool_choose_tls_mode "$DOMAIN" "$TLS_MODE")"
SITE="$(gopool_site_address "$DOMAIN" "$TLS_MODE")"
RESOLVED_IP="$(gopool_resolve_ipv4 "$DOMAIN")"
PRIMARY_IP="$(gopool_primary_ipv4)"
gopool_set_env_var .env GOPOOL_DOMAIN "$DOMAIN"
gopool_set_env_var .env GOPOOL_SITE "$SITE"
gopool_set_env_var .env GOPOOL_IMAGE "$GOPOOL_DEFAULT_IMAGE"
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
  local services=(gopool gopool-api caddy)
  if [ -f .secrets/node-config.toml ]; then
    services+=(gopool-validator)
  else
    echo "No node-config.toml: starting without the validator node."
  fi
  docker compose pull "${services[@]}"
  docker compose up -d "${services[@]}"
}

if [ "$SKIP_START" = 1 ]; then
  echo "Skipped start (--skip-start)."
else
  start_stack
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
  cd ${INSTALL_DIR} && docker compose logs -f
EOF
