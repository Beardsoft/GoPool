# Devnet + GoPool

Run the full Nimiq Albatross devnet with 4 validators and GoPool in one command.

## Prerequisites

- Docker and Docker Compose v2
- `NETWORK_NAME` defaults to `nimiq.local` (set via Makefile)

## Quick start

```bash
# From project root
make dev-up
```

This will:
1. Build the Albatross images (`devnet-build`)
2. Create missing devnet-only secrets and seed `devlab/.runtime/config/config.json`
3. Start the devnet (traefik + seed1-4) and GoPool services

## Makefile targets

- `make devnet-build` – build albatross images
- `make devnet-prepare` – create missing devnet secrets and runtime config without overwriting existing files
- `make devnet-up` – start devnet + GoPool (builds images)
- `make devnet-down` – stop everything
- `make devnet-logs` – follow logs
- `make dev-up` – build + up
- `make dev-down` – down

## Compose files

- `devlab/docker-compose.yaml` – Albatross devnet
- `devlab/docker-compose.pool.yml` – GoPool daemon + API attached to the devnet
- `devlab/config.dev.json` – pool config pointing to `http://seed1:8648`

Services:

- `seed1` – RPC exposed on host `8647:8648`, validator `NQ20TSB0...`
- `gopool-dev` – daemon, metrics `:9100`, DB mounted from `../data`
- `gopool-api-dev` – API on host `52413:8080`

## Manual compose

```bash
cd devlab
NETWORK_NAME=nimiq.local docker compose -f docker-compose.yaml -f docker-compose.pool.yml up --build -d
```

## Notes

- The pool uses `devlab/config.dev.json` with `rpc_url: http://seed1:8648`
- Preparation uses seed1's public devnet account fixture; never reuse it outside this local devnet
- Data volume `../data` is shared with the host
- Operator console auth: add your Hub address to `operator_addresses` in `devlab/.runtime/config/config.json`, then restart `gopool-api-dev`. The validator address is always allowed; an empty list means only that validator can sign in.
- To stop: `make dev-down` or `docker compose -f devlab/docker-compose.yaml -f devlab/docker-compose.pool.yml down`
