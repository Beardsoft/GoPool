# GoPool

GoPool is a Go-based Nimiq Albatross validator pool daemon. It tracks delegators per epoch, records rewards from the chain, and pays out stake proportionally using SQLite for persistence and RPC for on-chain interaction.

## Features

- Tracks user participation per epoch via `getValidatorByAddress` / `getStakersByValidatorAddress`.
- Records rewards from `getInherentsByBlockNumber` at checkpoint blocks.
- Pays out rewards as stake or transfers after a configurable minimum.
- Pool fee wallet and percentage support.
- Auto-reactivate, validator CLI actions, Prometheus metrics, and a REST API.
- Real-time SSE stream at `/api/events` for epoch starts and checkpoint rewards, with live Vue dashboard.
- Operator alerts via Telegram/Webhook (including Discord webhook URLs) for validator state changes and payout failures.

## Prerequisites

- Go 1.25+
- SQLite
- Docker + Docker Compose for the devnet
- A Nimiq RPC endpoint

## Quick start

One command on a fresh Ubuntu/Debian VPS:

```bash
curl -fsSL https://raw.githubusercontent.com/Beardsoft/GoPool/master/scripts/install.sh \
  | sudo bash -s -- --domain pool.example.com
```

That clones into `/opt/gopool`, installs Docker if needed, generates a validator wallet, pulls `ghcr.io/beardsoft/gopool:latest`, and starts the daemon + API + validator node behind Caddy. It prints a setup URL that already includes the one-time token — open it, confirm name/fee/operators, and launch. The API writes the configuration and activates in-process; no Compose restart.

**You still do by hand:**

1. Point DNS at the VPS (`A` record) **or** add a reverse-proxy host that forwards `https://your.domain` to `http://VPS:80`.
2. On a public VPS, open ports `80` and `443` in the cloud firewall. The installer opens `ufw` when it is active.
3. Open the printed `/setup?token=…` link, confirm the pre-filled wallet/network, set the pool name/fee, and launch.
4. Back up `.secrets/wallet.json` offline, then consider deleting it from the server.
5. Stake or register the validator on chain. A `readiness_error` is expected until that exists.

If the domain already hits Nginx Proxy Manager, Traefik, or Cloudflare (it does not resolve to this machine), the installer serves HTTP on port `80` and tells you the forward target. Force either mode with `--tls auto` or `--tls proxy`.

For testnet, add `--network test-albatross`. To install somewhere other than `/opt/gopool`, add `--dir /path`.

### Local / from a git checkout

```bash
git clone https://github.com/Beardsoft/GoPool.git
cd GoPool
go mod tidy
```

Create deployment-only secrets and the writable configuration directory:

```bash
install -d -m 700 .secrets data/config
openssl rand -hex 32 > .secrets/setup-token
openssl rand -hex 32 > .secrets/session-secret
install -m 600 /dev/stdin .secrets/validator-key <<<'<validator-private-key-hex>'
# Validator node config (needs the full key set in .secrets/wallet.json):
./scripts/make-validator-node-config.sh --network main-albatross
echo 'GOPOOL_DOMAIN=your.pool.domain' > .env   # omit for local testing, defaults to localhost
docker compose --env-file .env -f deployments/docker-compose.yml up -d
```

The stack also runs `gopool-validator`, the validator node itself (it produces the blocks the pool earns on). Without a full key set (`.secrets/wallet.json`) you can start just the pool: `docker compose -f deployments/docker-compose.yml up -d gopool gopool-api caddy`.

The API sits behind a bundled Caddy reverse proxy that handles TLS automatically (Let's Encrypt for a real domain, a locally-trusted cert for `localhost`). The API container itself only binds to `127.0.0.1`; Caddy on ports `80`/`443` is the public entrypoint.

Open `https://localhost` (or `https://your.pool.domain/setup?token=…` after the installer), complete the browser assistant, and wait until the daemon heartbeat matches. A `readiness_error` can remain if the configured address is not yet a validator on chain.

## VPS onboarding

`scripts/install.sh` is the one-shot path above. From a checkout, the same wizard is:

```bash
sudo ./scripts/vps-onboard.sh --yes --domain pool.example.com --generate-wallet
```

Omit `--yes` for the interactive version (wallet generate vs paste, domain, start). The script installs Docker if missing, generates the setup/session secrets, and either generates a brand-new validator wallet (via the official `ghcr.io/nimiq/core-rs-albatross` image — no separate signup or tooling needed) or lets you paste in an existing payout private key. With a full wallet it writes `.secrets/node-config.toml` for the validator node (default network `main-albatross`) and `data/config/setup-hints.json` so the browser assistant is pre-filled. It then starts the daemon + API + validator node behind a bundled Caddy reverse proxy, pulling `ghcr.io/beardsoft/gopool` instead of compiling on the VPS.

Caddy terminates TLS with Let's Encrypt when the domain resolves to this server. If it does not (existing reverse proxy), Caddy serves HTTP on port `80` and the installer prints the forward address. `52412` is loopback-only; `9100` (metrics) should stay internal unless you scrape it remotely.

If you generate a new wallet, the full key set (address, signing, fee, and BLS voting keys — everything needed to also run the validator node itself, not just GoPool) is written to `.secrets/wallet.json`. Back it up offline and consider deleting it from the server afterward; GoPool itself only ever reads `.secrets/validator-key` (the payout address's private key). Pasting only a payout key starts the pool without the validator node — add the full key set to `.secrets/wallet.json` and re-run the wizard (or `make-validator-node-config.sh` + `docker compose up -d`) to enable it. The validator node syncs the chain on first start and produces blocks once the pool has staked it.

It prints the setup URL (token included as `?token=`) at the end — open it and complete the browser assistant. The daemon waits for `config.json`, retries validator readiness, and reloads when the file hash changes. No Compose restart is required after setup or Settings saves.

For Swarm clusters instead of a single VPS, see [deployments/SWARM.md](deployments/SWARM.md). Homelab testnet (`test-albatross`, RPC `https://rpc-testnet.nimiqscan.com`) is `https://pool-testnet.maestroi.cc`. Generate a validator wallet with `./scripts/generate-validator-wallet.sh` before creating the Docker secrets.

## Configuration

Configuration is loaded from `CONFIG_FILE` (default `config.json`). Settings — including alert credentials — are revisioned and atomically written by the API. The validator key and session secret remain deployment-managed.

Key fields:

- `rpc_url` – Nimiq RPC HTTP endpoint
- `network` – `dev-albatross`, `test-albatross`, or `main-albatross`
- `pool_fee_wallet` – address receiving pool fees
- `pool_fee_percentage` – 0.0-1.0
- `payout_mode` – `delegate` or `transfer`
- `min_payout_luna` – minimum payout threshold
- `auto_reactivate` – boolean
- `metrics_addr` – Prometheus listener, e.g. `:9100`
- `api_addr` and `validator_address` – public API/validator identity
- `POOL_PRIVATE_KEY_FILE` – daemon-only validator key file
- `POOL_SETUP_TOKEN_FILE` and `POOL_SESSION_SECRET_FILE` – API-only bootstrap/session files
- alert channels are fully self-service: destinations and credentials (Telegram token, webhook URL) are set in the UI and never returned by the API

Example dev config is in `config/config.json-example`. The repo ships `config.json` for local dev.

## Running the pool

### Daemon

```bash
go run ./cmd
```

The daemon loads the non-secret config plus `POOL_PRIVATE_KEY_FILE`, connects to RPC, opens SQLite, and starts the main loop. It refuses to sign without its daemon-only key file.

Select an alternate non-secret configuration file:

```bash
CONFIG_FILE=devlab/config.dev.json \
POOL_PRIVATE_KEY_FILE=.secrets/validator-key go run ./cmd
```

Metrics are exposed on `metrics_addr` if set.

### API server

The REST API is in `cmd/api`. With no config it starts in setup-only mode. Once configured it reads `POOL_SESSION_SECRET_FILE`, never mounts the validator key, and exposes the public/operator application.

```bash
go run ./cmd/api
```

API listens on `cfg.APIAddr` and uses the same SQLite DB.

Key endpoints:
- `GET /api/pool` – pool status
- `GET /api/epochs` / `GET /api/epochs/{number}` – epoch details and stakers
- `GET /api/epochs/{number}/rewards` – rewards per batch for an epoch
- `GET /api/events` – Server-Sent Events stream for real-time updates: `epoch_started`, `checkpoint_reward`, `payout_sent`

### Validator CLI

```bash
go run ./cmd validator deactivate
go run ./cmd validator retire
go run ./cmd validator delete <recipient_address> <value-luna>
```

## Running devlab

The `devlab/` directory contains a Docker Compose devnet with 4 Albatross validators and Traefik.

Prerequisites: Docker, `NETWORK_NAME` env var defaults to `nimiq.local`.

### Manual scripts

```bash
cd devlab
./run.sh sync          # clone albatross repo if needed
./run.sh build-albatross
./run.sh up-albatross  # starts traefik + seed1-4
./run.sh log-albatross
./run.sh down-albatross
```

RPC is exposed on host port `8647` → `seed1`. Update `config.json`:

```json
{
  "rpc_url": "http://127.0.0.1:8647",
  "network": "dev-albatross",
  ...
}
```

Then start GoPool as above. The devnet uses pre-funded validator addresses defined in `devlab/docker-compose.yaml`.

### Makefile – devnet + pool in one

A combined devnet + GoPool setup is provided via `Makefile`. It builds the Albatross images and starts the devnet together with GoPool daemon and API.

```bash
make dev-up          # build albatross + start devnet + GoPool
make devnet-up       # start devnet + GoPool, builds images
make devnet-build    # build albatross images only
make devnet-down     # stop devnet + GoPool
make devnet-logs     # follow logs
make dev-down        # stop devnet + GoPool
```

The Makefile uses `devlab/docker-compose.yaml` + `devlab/docker-compose.pool.yml`. Traefik is exposed on host ports `8444` and `8649` to avoid conflicts, seed1 RPC remains on `8647`. The pool uses `devlab/config.dev.json` with `rpc_url: http://seed1:8648`.

Example manual compose:

```bash
NETWORK_NAME=nimiq.local docker compose -f devlab/docker-compose.yaml -f devlab/docker-compose.pool.yml up --build -d
```

## Docker deployment

A production Compose example is in `deployments/docker-compose.yml`. It pulls `ghcr.io/beardsoft/gopool:latest` by default (`GOPOOL_IMAGE` to pin a SHA) and enforces the daemon/API secret boundary and distinct read-only/read-write configuration mounts.

Build and run locally (optional; VPS installs pull the published image):

```bash
docker compose --env-file .env -f deployments/docker-compose.yml up --build
```

Images for Swarm are `ghcr.io/beardsoft/gopool:<git-sha>` (published on every `master` push) and version tags from GitHub Releases. Settings saves and revision restores apply in-process; the UI shows Activating until the daemon heartbeat reports the expected hash. Force-update services only when rolling out a new image. For Swarm rotation and failed-readiness recovery, see [deployments/SWARM.md](deployments/SWARM.md).

## Project structure

- `cmd/` – daemon entrypoint and API server
- `internal/pool/` – election, checkpoint, payout, confirmation logic
- `internal/chain/` – RPC client
- `internal/db/` – SQLite schema and sqlc queries
- `internal/api/` – REST handlers
- `internal/config/` and `internal/configstore/` – signer-aware loading and atomic revisions
- `devlab/` – Docker devnet scripts and compose
- `web/` – frontend UI

## Development

Run tests:

```bash
go test ./...
```

Format:

```bash
go fmt ./...
```

The DB schema is in `schema/scheme.sql` and generated queries in `internal/db/queries.sql.go`.

See `DESIGN.md` for the pool algorithm overview.
