# Docker Swarm deployment

GoPool uses three separate secrets. Only the daemon receives the validator key; only the API receives setup and session secrets. Both services share SQLite and a configuration volume, mounted read-only in the daemon and writable in the API.

Images are published to GHCR on every push to `master` (`ghcr.io/beardsoft/gopool:<full-sha>` and `:latest`) and on version tags (`:vX.Y.Z`). Pin deploys to the full commit SHA. The package is public; `docker stack deploy --with-registry-auth` is still useful if you pin a private override image.

Attach the API to the cluster's existing Traefik overlay named `web`. SQLite volumes are local: pin both replicas to one hostname. The test stack uses `vm-swarm-worker-01` (same node as Traefik); `node.role == manager` is wrong here because several nodes are managers and overlay to the leader does not reach Traefik.

## Generate keys and bootstrap secrets

```bash
# Full validator identity (address + signing/fee/BLS). Back up wallet.json offline.
./scripts/generate-validator-wallet.sh --out-dir .secrets/testnet
# The validator node's client.toml (contains signing/fee/voting keys).
./scripts/make-validator-node-config.sh --out-dir .secrets/testnet --network test-albatross

openssl rand -hex 32 > .secrets/testnet/setup-token
openssl rand -hex 32 > .secrets/testnet/session-secret
chmod 600 .secrets/testnet/setup-token .secrets/testnet/session-secret
```

GoPool reads the payout address private key (`validator-key`) and, for auto-register, `wallet.json` (signing + voting keys). The rest of `wallet.json` also goes into `node-config.toml` for `gopool-validator`. That node produces blocks. The pool uses the configured public RPC, with the validator node's internal RPC as a fallback and as the only endpoint for CreateValidator.

## Homelab testnet (`gopool-test`)

Public UI: `https://pool-testnet.maestroi.cc`
RPC (set in the browser assistant): `https://rpc-testnet.nimiqscan.com`
Network: `test-albatross`

On the Swarm manager (`root@192.168.50.151`):

```bash
# Once per cluster (skip if already logged in)
echo "$GHCR_TOKEN" | docker login ghcr.io -u "$GHCR_USER" --password-stdin

# Create secrets once. Recreating a named secret fails; rotate by using a new name.
docker secret create gopool_test_setup_token .secrets/testnet/setup-token
docker secret create gopool_test_session_secret .secrets/testnet/session-secret
docker secret create gopool_test_validator_key .secrets/testnet/validator-key
docker secret create gopool_test_wallet_json .secrets/testnet/wallet.json
docker secret create gopool_test_node_config .secrets/testnet/node-config.toml

SHA=$(git rev-parse HEAD)
GOPOOL_IMAGE=ghcr.io/beardsoft/gopool:${SHA} \
  docker stack deploy --with-registry-auth -c deployments/docker-stack-test.yml gopool-test
```

Open `https://pool-testnet.maestroi.cc`, enter the setup token, and complete the assistant. An unconfigured daemon serves only the setup wizard (`/` redirects to `/setup`). Use:

- RPC URL `https://rpc-testnet.nimiqscan.com`
- Network `test-albatross`
- Validator address from `wallet.json`

The daemon waits for `config.json` instead of crashing. After the assistant writes the revision, the API swaps to full routes and the daemon reloads between payout ticks. Watch the heartbeat; do not force-update the services after setup:

```bash
docker service logs --tail 100 gopool-test_gopool
```

Readiness is complete when the daemon heartbeat reports the new configuration hash and derived validator address. On testnet the daemon faucets ~101k NIM, registers the validator, and self-stakes leftover. On mainnet send ≥101k NIM to the address first. A `readiness_error` remains until that finishes. Force-update (`docker service update --force`) only when rolling out a new image.

The `gopool-validator` service syncs the chain on first start (watch `docker service logs gopool-test_gopool-validator`). Its RPC is internal only. Once registered and self-staked, it produces blocks; `automatic_reactivate` in its config re-activates it if it ever goes inactive.

The generated `node-config.toml` must keep `network_buffer_size > 0` (some Nimiq images default it to 0, which panics the validator's request handlers) and `seed_nodes` (without them the node never bootstraps the DHT and stays at 0 peers). It is a pruned full node (`sync_mode = "full"`, `max_epochs_stored = 2`, `index_history = false`): validators keep the current state tree, not complete transaction history. The Nimiq client default is `history`, which fills the disk. Switching from history to full uses a separate database name, so the node resyncs; delete the old history database if you are migrating. `make-validator-node-config.sh` sets these; if you swap the node image, re-check them against the new release. The stack defaults to `ghcr.io/nimiq/core-rs-albatross:latest`. Watchtower is a Compose-only helper on the VPS installer; it does not belong on Swarm (it restarts containers, and Swarm then reconciles them back to the service spec). Pin SHA tags and `docker service update` / `stack deploy` instead.

## Generic / production stack

Same flow with `deployments/docker-stack.yml` and unprefixed secret names:

```bash
openssl rand -hex 32 | docker secret create gopool_setup_token -
openssl rand -hex 32 | docker secret create gopool_session_secret -
printf '%s' "$(cat .secrets/validator-key)" | docker secret create gopool_validator_key -
docker secret create gopool_wallet_json .secrets/wallet.json
docker secret create gopool_node_config .secrets/node-config.toml

GOPOOL_IMAGE=ghcr.io/beardsoft/gopool:$(git rev-parse HEAD) \
GOPOOL_DOMAIN=your.pool.domain \
  docker stack deploy --with-registry-auth -c deployments/docker-stack.yml gopool
```

After setup, wait for the heartbeat. Testnet faucets/registers/self-stakes on its own; mainnet waits for ≥101k NIM on the address. Force-update `gopool_gopool` and `gopool_gopool-api` only when deploying a new image.

## Rotation and recovery

- Rotate a Docker secret by creating a new uniquely named secret, updating the stack reference, deploying, then removing the old secret after both services converge.
- The setup token is unusable after setup completion and may be removed from the API service in a follow-up stack revision.
- Restore a prior non-secret revision from Operator → Settings and wait for the matching heartbeat. The daemon reloads the file in-process when payouts are idle.
- If readiness fails, do not retry on-chain actions. Check the daemon logs, RPC/network pairing, validator-address mismatch, key-file permissions, and the pending configuration hash first.
- SQLite and the configuration volume must stay on the manager selected by the placement constraint.
