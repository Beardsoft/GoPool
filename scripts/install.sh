#!/usr/bin/env bash
# One-shot VPS installer. Intended to be curled:
#   curl -fsSL https://raw.githubusercontent.com/Beardsoft/GoPool/master/scripts/install.sh \
#     | sudo bash -s -- --domain pool.example.com
set -euo pipefail

REPO="${GOPOOL_REPO:-https://github.com/Beardsoft/GoPool.git}"
BRANCH="${GOPOOL_BRANCH:-master}"
INSTALL_DIR="${GOPOOL_DIR:-/opt/gopool}"
DOMAIN=""
NETWORK="main-albatross"
RPC_URL=""
GENERATE_WALLET=1
TLS_MODE=""

usage() {
  cat <<EOF
Install GoPool on a fresh Ubuntu/Debian VPS (Docker Compose, not Swarm).

  curl -fsSL https://raw.githubusercontent.com/Beardsoft/GoPool/master/scripts/install.sh \\
    | sudo bash -s -- --domain pool.example.com

Point a DNS A record at the server and open ports 80 and 443 first.
If the domain already hits Nginx Proxy Manager, Traefik, or Cloudflare,
the installer detects that and serves HTTP on :80 for the proxy to
forward. Override with --tls auto or --tls proxy.

The script clones the repo, installs Docker if needed, generates a validator
wallet, pulls ghcr.io/beardsoft/gopool, and prints a setup URL with the token.

Options:
  --domain NAME     DNS name for the pool (required)
  --network NAME    main-albatross (default) or test-albatross
  --rpc-url URL     pool RPC endpoint (defaults from --network)
  --tls auto|proxy  auto: Caddy + Let's Encrypt; proxy: HTTP behind TLS proxy
  --dir PATH        install directory (default /opt/gopool)
  --branch NAME     git branch to clone (default master)
  --no-wallet       do not generate a wallet (you must add .secrets/validator-key)
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
    --branch) BRANCH="$2"; shift ;;
    --no-wallet) GENERATE_WALLET=0 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage >&2; exit 1 ;;
  esac
  shift
done

if [ -z "$DOMAIN" ]; then
  echo "Missing --domain. Example:" >&2
  echo "  curl -fsSL https://raw.githubusercontent.com/Beardsoft/GoPool/master/scripts/install.sh | sudo bash -s -- --domain pool.example.com" >&2
  exit 1
fi
if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root (sudo)." >&2
  exit 1
fi

if ! command -v git >/dev/null 2>&1; then
  apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git ca-certificates
fi

if [ -d "$INSTALL_DIR/.git" ]; then
  echo "Updating ${INSTALL_DIR}..."
  git -C "$INSTALL_DIR" fetch --depth 1 origin "$BRANCH"
  git -C "$INSTALL_DIR" checkout -q -B "$BRANCH" FETCH_HEAD
elif [ -e "$INSTALL_DIR" ] && [ ! -f "$INSTALL_DIR/scripts/vps-onboard.sh" ]; then
  echo "${INSTALL_DIR} exists and is not a GoPool checkout. Pass --dir." >&2
  exit 1
elif [ ! -f "$INSTALL_DIR/scripts/vps-onboard.sh" ]; then
  git clone --depth 1 --branch "$BRANCH" "$REPO" "$INSTALL_DIR"
fi

cd "$INSTALL_DIR"
args=(--yes --domain "$DOMAIN" --network "$NETWORK")
[ -n "$RPC_URL" ] && args+=(--rpc-url "$RPC_URL")
[ -n "$TLS_MODE" ] && args+=(--tls "$TLS_MODE")
if [ "$GENERATE_WALLET" = 1 ]; then
  args+=(--generate-wallet)
else
  args+=(--no-wallet)
fi
exec ./scripts/vps-onboard.sh "${args[@]}"
