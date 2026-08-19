# Contributing to GoPool

Pool operators should start with the [README](README.md). This file is for people changing the code.

## Prerequisites

- Go 1.25+
- Node 20+ (frontend)
- Docker + Docker Compose
- SQLite (used via CGO)

## Clone and tests

```bash
git clone https://github.com/Beardsoft/GoPool.git
cd GoPool
go test ./...
cd web && npm ci && npx vitest run && npm run build
```

Installer helpers: `bash scripts/vps-onboard_test.sh`.

Schema lives in `schema/scheme.sql`; sqlc queries in `sql/queries.sql` (`sqlc generate`).

## Local stack from a git checkout

```bash
install -d -m 700 .secrets data/config
openssl rand -hex 32 > .secrets/setup-token
openssl rand -hex 32 > .secrets/session-secret
install -m 600 /dev/stdin .secrets/validator-key <<<'<validator-private-key-hex>'
./scripts/make-validator-node-config.sh --network main-albatross   # needs .secrets/wallet.json
echo 'GOPOOL_DOMAIN=localhost' > .env
docker compose --env-file .env -f deployments/docker-compose.yml up -d
```

Without a full wallet you can start only the pool: `docker compose -f deployments/docker-compose.yml up -d gopool gopool-api caddy`.

Open `https://localhost` and complete setup. Pin images with `GOPOOL_IMAGE=ghcr.io/beardsoft/gopool:<tag>`.

From a checkout, the interactive VPS helper is `sudo ./scripts/vps-onboard.sh --domain pool.example.com --generate-wallet` (omit `--yes` to be prompted). The public curl|bash path is `scripts/install.sh` and does not clone this repo.

## Run binaries on the host

```bash
CONFIG_FILE=config.json POOL_PRIVATE_KEY_FILE=.secrets/validator-key go run ./cmd
go run ./cmd/api
go run ./cmd validator deactivate
go run ./cmd validator retire
go run ./cmd validator delete <recipient_address> <value-luna>
```

Example config: `config/config.json-example`. Alerts are set in the UI, not in the key file.

Public API includes `GET /api/pool`, epoch routes, and operator SSE at `GET /api/operator/events`. Setup uses a one-time token; the validator key is never mounted in the API container.

## Devnet

See [devlab/DEVNET.md](devlab/DEVNET.md). Short path: `make dev-up`.

## Layout

- `cmd/` — daemon and API
- `internal/pool/` — election, payout, autostake
- `internal/chain/` — RPC client
- `internal/db/` — SQLite + sqlc
- `internal/api/` — HTTP handlers
- `web/` — Vue UI
- `deployments/` — Compose, Caddy, Swarm
- `scripts/install.sh` — one-shot VPS install
