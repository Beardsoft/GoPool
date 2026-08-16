#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

RPC_URL="${RPC_URL:-http://127.0.0.1:8647}"
MAX_RETRIES=5
RETRY_DELAY=5

log() { echo "[guard-devnet] $*"; }

check_rpc() {
  local resp
  if ! resp=$(timeout 5 bash -c "curl -s -X POST -H 'Content-Type: application/json' --data '{\"jsonrpc\":\"2.0\",\"method\":\"getBlockNumber\",\"params\":[],\"id\":1}' \"$RPC_URL\"" 2>/dev/null); then
    return 1
  fi
  if echo "$resp" | grep -q '"result"'; then
    return 0
  fi
  return 1
}

log "Checking RPC at $RPC_URL"
for i in $(seq 1 $MAX_RETRIES); do
  if check_rpc; then
    log "RPC responsive"
    exit 0
  fi
  log "RPC not responsive, attempt $i/$MAX_RETRIES"
  sleep $RETRY_DELAY
done

log "RPC unhealthy, attempting devnet reset"
docker compose -f docker-compose.yaml down -v || true
# Also remove volume data if present
docker compose -f docker-compose.yaml up -d
log "Devnet restarted, waiting for RPC"
sleep 10
for i in $(seq 1 $MAX_RETRIES); do
  if check_rpc; then
    log "RPC restored"
    exit 0
  fi
  log "Waiting for RPC, attempt $i/$MAX_RETRIES"
  sleep $RETRY_DELAY
done

log "Devnet still unhealthy after reset"
exit 1
