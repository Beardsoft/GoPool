#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
# shellcheck source=install.sh
source scripts/install.sh

fail() { echo "FAIL: $*" >&2; exit 1; }

[ "$(gopool_rpc_url_for_network main-albatross)" = "https://rpc-mainnet.nimiqscan.com" ] || fail "main rpc"
[ "$(gopool_rpc_url_for_network test-albatross)" = "https://rpc-testnet.nimiqscan.com" ] || fail "test rpc"

[ "$(gopool_setup_url pool.example.com abcdef)" = "https://pool.example.com/setup?token=abcdef" ] || fail "setup url"

[ "$(gopool_site_address pool.example.com auto)" = "pool.example.com" ] || fail "site auto"
[ "$(gopool_site_address pool.example.com proxy)" = "http://pool.example.com" ] || fail "site proxy"
[ "$(gopool_choose_tls_mode pool.example.com auto)" = "auto" ] || fail "tls override auto"
[ "$(gopool_choose_tls_mode pool.example.com proxy)" = "proxy" ] || fail "tls override proxy"
[ "$(gopool_choose_tls_mode localhost)" = "auto" ] || fail "tls localhost"
if gopool_choose_tls_mode pool.example.com bogus >/dev/null 2>&1; then
  fail "tls bogus should fail"
fi
gopool_domain_points_here "" && fail "empty resolve should not point here"
gopool_domain_points_here 203.0.113.1 && fail "TEST-NET IP should not point here"

dir="$(mktemp -d)"
gopool_write_caddyfile "$dir/Caddyfile"
grep -q 'GOPOOL_SITE' "$dir/Caddyfile" || fail "caddyfile site"
gopool_write_compose "$dir/compose.yml"
grep -q 'ghcr.io/beardsoft/gopool' "$dir/compose.yml" || fail "compose image"
grep -q 'core-rs-albatross:latest' "$dir/compose.yml" || fail "compose nimiq latest"
grep -q 'build:' "$dir/compose.yml" && fail "compose must not build on the VPS"
grep -q 'git clone' "$dir/compose.yml" && fail "compose must not clone"
grep -q './.secrets/validator-key' "$dir/compose.yml" || fail "compose secrets path"
grep -q 'name: gopool' "$dir/compose.yml" || fail "compose project name"
grep -q '/dev/tcp/127.0.0.1/8080' "$dir/compose.yml" || fail "api healthcheck"
grep -q '/dev/tcp/127.0.0.1/9100' "$dir/compose.yml" || fail "daemon healthcheck"
grep -q 'nickfedor/watchtower' "$dir/compose.yml" || fail "watchtower image"
grep -q '/var/run/docker.sock:/var/run/docker.sock' "$dir/compose.yml" || fail "watchtower docker.sock"
grep -q 'WATCHTOWER_LABEL_ENABLE' "$dir/compose.yml" || fail "watchtower label filter"
grep -q 'WATCHTOWER_SCHEDULE' "$dir/compose.yml" || fail "watchtower schedule"
grep -q 'com.centurylinklabs.watchtower.enable=true' "$dir/compose.yml" || fail "watchtower enable labels"
grep -q 'caddy watchtower' scripts/install.sh || fail "install starts watchtower"

wallet="$dir/wallet.json"
cat > "$wallet" <<'JSON'
{
  "validator_address": "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E",
  "signing_private_key": "aa",
  "fee_private_key": "bb",
  "voting_secret_key": "cc"
}
JSON
gopool_write_node_config "$wallet" "test-albatross" "$dir/node-config.toml"
grep -q 'network = "test-albatross"' "$dir/node-config.toml" || fail "node network"
grep -q 'seed1.pos.nimiq-testnet.com' "$dir/node-config.toml" || fail "node seeds"
grep -q 'up -d --pull missing' scripts/install.sh || fail "compose must pull only missing images"
grep -q 'sync_mode = "full"' "$dir/node-config.toml" || fail "pruned full sync_mode"
grep -q 'max_epochs_stored = 2' "$dir/node-config.toml" || fail "max_epochs_stored"
grep -q 'index_history = false' "$dir/node-config.toml" || fail "index_history"
[ "$(stat -c %a "$dir/node-config.toml")" = 644 ] || fail "node-config mode"
rm -rf "$dir"

envf="$(mktemp)"
gopool_set_env_var "$envf" GOPOOL_DOMAIN pool.example.com
gopool_set_env_var "$envf" GOPOOL_SITE "http://pool.example.com"
gopool_set_env_var "$envf" GOPOOL_SITE "pool.example.com"
grep -q '^GOPOOL_SITE=pool.example.com$' "$envf" || fail "site replace"
grep -c '^GOPOOL_SITE=' "$envf" | grep -qx 1 || fail "duplicate site"
rm -f "$envf"

tmp="$(mktemp)"
gopool_write_setup_hints "$tmp" "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E" "main-albatross" "https://rpc-mainnet.nimiqscan.com"
python3 - "$tmp" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
assert d["validator_address"].startswith("NQ20")
assert d["pool_fee_wallet"].startswith("NQ20")
assert d["network"] == "main-albatross"
assert d["rpc_url"] == "https://rpc-mainnet.nimiqscan.com"
PY
rm -f "$tmp"

help_out="$(bash scripts/install.sh --help)"
printf '%s\n' "$help_out" | grep -q 'Pulls published images' || fail "install help should say it pulls images"
pipe_help="$(bash -s -- --help < scripts/install.sh)"
printf '%s\n' "$pipe_help" | grep -q 'Pulls published images' || fail "piped install.sh --help (curl|bash)"
if bash scripts/install.sh >/dev/null 2>&1; then
  fail "install.sh should require --domain"
fi
if bash scripts/vps-onboard.sh --yes >/dev/null 2>&1; then
  fail "vps-onboard.sh --yes should require --domain"
fi
if ! grep -q 'BASH_SOURCE\[0\]:-' scripts/install.sh; then
  fail "install.sh must tolerate unset BASH_SOURCE"
fi

echo OK
