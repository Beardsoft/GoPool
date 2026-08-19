#!/usr/bin/env bash
# Non-interactive validator wallet generator. Writes the full key set to
# $OUT_DIR/wallet.json and the payout private key to $OUT_DIR/validator-key.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=install.sh
source "$SCRIPT_DIR/install.sh"

OUT_DIR="${OUT_DIR:-$(cd "$SCRIPT_DIR/.." && pwd)/.secrets}"
FORCE=0

while [ $# -gt 0 ]; do
  case "$1" in
    --force) FORCE=1 ;;
    --out-dir) OUT_DIR="$2"; shift ;;
    *) echo "Unknown argument: $1" >&2; exit 1 ;;
  esac
  shift
done

if [ -f "$OUT_DIR/wallet.json" ] && [ "$FORCE" -ne 1 ]; then
  echo "Refusing to overwrite $OUT_DIR/wallet.json (pass --force)." >&2
  exit 1
fi
gopool_generate_wallet "$OUT_DIR"
