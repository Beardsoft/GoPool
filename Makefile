.PHONY: help up down restart logs logs-f build daemon api devnet-prepare devnet-up devnet-down devnet-logs devnet-build dev-up dev-down devnet-restart dev-reset dev-clean dev-stakers dev-api-rebuild

COMPOSE_FILE := deployments/docker-compose.yml
DEVNET_DIR := devlab
DEVNET_COMPOSE := $(DEVNET_DIR)/docker-compose.yaml
DEVNET_POOL_COMPOSE := $(DEVNET_DIR)/docker-compose.pool.yml
NETWORK_NAME ?= nimiq.local
STAKER_COUNT ?= 20
STAKER_FUND_NIM ?= 6000
STAKER_STAKE_NIM ?= 5000
FAUCET_PRIV_KEY ?= 3336f25f5b4272a280c8eb8c1288b39bd064dfb32ebc799459f707a0e88c4e5f
VALIDATOR_ADDR ?= NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E

help:
	@echo "GoPool Makefile"
	@echo "  make up          - Build and start daemon + api"
	@echo "  make down        - Stop and remove containers"
	@echo "  make restart     - Restart services"
	@echo "  make logs        - Follow logs"
	@echo "  make logs-once   - Show logs once"
	@echo "  make build       - Build images without starting"
	@echo "  make daemon      - Run daemon locally via go run"
	@echo "  make api         - Run API locally via go run"
	@echo ""
	@echo "Devnet targets:"
	@echo "  make devnet-build   - Build albatross images"
	@echo "  make devnet-prepare - Create missing devnet secrets and config"
	@echo "  make devnet-up      - Start devnet + GoPool (build)"
	@echo "  make devnet-down    - Stop devnet + GoPool"
	@echo "  make devnet-logs    - Follow devnet logs"
	@echo "  make dev-up         - Build albatross then start devnet + GoPool"
	@echo "  make dev-down       - Stop devnet + GoPool"
	@echo "  make dev-reset      - Stop devnet + GoPool and wipe data volume"
	@echo "  make dev-stakers    - Create test stakers for devnet"
	@echo "                       STAKER_COUNT=$(STAKER_COUNT) STAKER_FUND_NIM=$(STAKER_FUND_NIM) STAKER_STAKE_NIM=$(STAKER_STAKE_NIM)"
	@echo "  make dev-api-rebuild - Rebuild and restart only gopool-api container"

up:
	docker compose -f $(COMPOSE_FILE) up --build -d

down:
	docker compose -f $(COMPOSE_FILE) down

restart:
	docker compose -f $(COMPOSE_FILE) restart

logs:
	docker compose -f $(COMPOSE_FILE) logs -f

logs-once:
	docker compose -f $(COMPOSE_FILE) logs

build:
	docker compose -f $(COMPOSE_FILE) build

daemon:
	go run ./cmd

api:
	go run ./cmd/api

devnet-build:
	cd $(DEVNET_DIR) && ./run.sh build-albatross

devnet-prepare:
	./$(DEVNET_DIR)/prepare.sh

devnet-up: devnet-prepare
	NETWORK_NAME=$(NETWORK_NAME) docker compose -f $(DEVNET_COMPOSE) -f $(DEVNET_POOL_COMPOSE) up --build -d

devnet-down:
	NETWORK_NAME=$(NETWORK_NAME) docker compose -f $(DEVNET_COMPOSE) -f $(DEVNET_POOL_COMPOSE) down

devnet-logs:
	NETWORK_NAME=$(NETWORK_NAME) docker compose -f $(DEVNET_COMPOSE) -f $(DEVNET_POOL_COMPOSE) logs -f

devnet-restart:
	NETWORK_NAME=$(NETWORK_NAME) docker compose -f $(DEVNET_COMPOSE) -f $(DEVNET_POOL_COMPOSE) restart

dev-up: devnet-build devnet-up

dev-down: devnet-down

dev-reset:
	NETWORK_NAME=$(NETWORK_NAME) docker compose -f $(DEVNET_COMPOSE) -f $(DEVNET_POOL_COMPOSE) down -v
	sudo rm -rf data && mkdir data

dev-clean: dev-reset

dev-stakers:
	$(DEVNET_DIR)/guard_devnet.sh
	go build -o /tmp/create_test_stakers ./devlab/create_test_stakers.go
	RPC_URL=http://127.0.0.1:8647 \
	NETWORK=dev-albatross \
	FAUCET_PRIV_KEY=$(FAUCET_PRIV_KEY) \
	VALIDATOR_ADDR="$(VALIDATOR_ADDR)" \
	COUNT=$(STAKER_COUNT) \
	FUND_NIM=$(STAKER_FUND_NIM) \
	STAKE_NIM=$(STAKER_STAKE_NIM) \
	/tmp/create_test_stakers

dev-api-rebuild:
	NETWORK_NAME=$(NETWORK_NAME) docker compose -f $(DEVNET_COMPOSE) -f $(DEVNET_POOL_COMPOSE) up --build -d gopool-api
