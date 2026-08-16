#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_root="${GOPOOL_DEV_ROOT:-$(cd "$script_dir/.." && pwd)}"
secret_dir="$project_root/.secrets"
config_dir="$project_root/devlab/.runtime/config"

install -d -m 700 "$secret_dir"
install -d -m 755 "$config_dir"

validator_key='6927eb8de74e8ea06a8afae5a66db176a7031f742b656651ac53bddb8a4ad3f3'
legacy_consensus_key='041580cc67e66e9e08b68fd9e4c9deb68737168fbe7488de2638c2e906c2f5ad'
validator_key_file="$secret_dir/validator-key"

if [[ ! -e "$validator_key_file" ]]; then
    printf '%s\n' "$validator_key" | install -m 600 /dev/stdin "$validator_key_file"
elif [[ -f "$validator_key_file" ]]; then
    IFS= read -r current_validator_key < "$validator_key_file" || true
    if [[ "$current_validator_key" == "$legacy_consensus_key" ]]; then
        printf '%s\n' "$validator_key" | install -m 600 /dev/stdin "$validator_key_file"
    fi
fi

for name in setup-token session-secret; do
    if [[ ! -e "$secret_dir/$name" ]]; then
        openssl rand -hex 32 | install -m 600 /dev/stdin "$secret_dir/$name"
    fi
done

if [[ ! -e "$config_dir/config.json" ]]; then
    install -m 644 "$script_dir/config.dev.json" "$config_dir/config.json"
fi
