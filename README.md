# GoPool

GoPool is a Go-based Nimiq Albatross validator pool daemon. It tracks delegators per epoch, records rewards from the chain, and pays out stake proportionally using SQLite for persistence and RPC for on-chain interaction.

## Features

- Tracks user participation per epoch via `getValidatorByAddress` / `getStakersByValidatorAddress`.
- Records rewards from `getInherentsByBlockNumber` at checkpoint blocks.
- Pays out rewards as stake or transfers after a configurable minimum.
- Pool fee wallet and percentage support.
- Auto-reactivate, validator CLI actions, Prometheus metrics, and a REST API.
- Real-time SSE stream at `/api/events` for epoch starts and checkpoint rewards, with live Vue dashboard.
- Operator alerts via Telegram/Webhook/Email for validator state changes and payout failures.

## Prerequisites

- Go 1.25+
- SQLite
- Docker + Docker Compose for the devnet
- A Nimiq RPC endpoint

## Quick start

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
docker compose -f deployments/docker-compose.yml up -d
```

Open `http://localhost:52412`, exchange the setup token, and complete the browser assistant. The API writes the full configuration, alert credentials included. Restart both services afterward and wait for daemon readiness.

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
- alert channels are fully self-service: destinations and credentials (Telegram token, SMTP host/port/username/password/from) are set in the UI and never returned by the API

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

A production Compose example is in `deployments/docker-compose.yml`. It enforces the daemon/API secret boundary and distinct read-only/read-write configuration mounts.

Build and run locally:

```bash
docker compose -f deployments/docker-compose.yml up --build
```

Settings saves and revision restores are pending until both services restart and the daemon heartbeat reports the expected hash. For Swarm rotation and failed-readiness recovery, see [deployments/SWARM.md](deployments/SWARM.md).

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
