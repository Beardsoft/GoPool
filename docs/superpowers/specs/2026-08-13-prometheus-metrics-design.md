# Daemon Prometheus metrics

## Context

GoPool is a 2s ticker daemon with zap logs and no HTTP. Production already
scrapes Nimiq nodes (`nimiq_blockchain_*`, `nimiq_staking_*`) in Grafana.
Those series say the chain is healthy; they do not say the pool is catching
up, paying stakers, or solvent.

The planned `cmd/api` operator health endpoint is a point-in-time UI in a
different process. Metrics belong on the daemon.

Rewards credit the validator reward address (`pool_fee_wallet`). Payouts
leave the signer (`private_key`). Those can be different accounts — a
known gap. Validator `Balance` is staked, not spendable.

## Architecture

The daemon binds a private HTTP server on `metrics_addr` (default `:9100`;
empty string disables). `GET /metrics` is Prometheus text; `GET /healthz`
is liveness only (always 200 if the process is up). Lag is an alert, not
an HTTP failure.

`prometheus/client_golang` on the default registry, including the default
Go and process collectors. Package `internal/metrics` holds package-level
collectors. `Manager` does not grow a metrics field.

No per-staker labels. No copies of `nimiq_blockchain_*`.

## Catalog

Prefix `gopool_`.

**Every tick** (in-memory or cheap SQL, no extra RPC):

- `gopool_chain_head` / `gopool_last_processed_height`
- `gopool_tick_duration_seconds` — last tick, gauge not histogram
- `gopool_stakers` / `gopool_delegated_stake_luna` — current
  `in_progress` epoch row (election snapshot)
- `gopool_payslips_pending` / `gopool_payslips_pending_luna` /
  `gopool_payslips_stuck` — stuck = `out_for_payment` +
  `awaiting_confirmation`. No age cutoff; `for: 15m` on the alert.

**Counters** next to existing log lines:

- `gopool_rewards_luna_total` / `gopool_pool_fee_luna_total` at checkpoint
  (full inherent value and the pool cut)
- `gopool_payouts_submitted_total{kind="delegate|transfer"}`
- `gopool_payouts_confirmed_total` / `gopool_payouts_failed_total`
- `gopool_rpc_errors_total{op=...}` — closed set: `block_number`,
  `height`, `payout`, `confirm`, `reactivate`, `balance`, `validator`

**Live, throttled (30s constant):**

- `gopool_live_stake_luna` / `gopool_live_stakers` from `GetValidator`
- `gopool_validator_state{state="active|inactive|jailed|retired"}` — 1/0
  enum, same inactive/jailed tests as `handleElection`
- `gopool_wallet_balance_luna{role="payout"}` — `GetBalance` on the signer
- `gopool_wallet_balance_luna{role="reward"}` — `GetBalance` on
  `pool_fee_wallet` only when it parses and differs from the signer

`GetValidator` already runs every auto-reactivate tick and at election.
Observing it there is free. The 30s throttle issues `GetValidator` only
when that observation is stale (auto-reactivate off, or the last fetch
older than 30s). `GetBalance` is always throttled: 1–2 extra RPCs per 30s,
never per 2s tick.

## Config

- `metrics_addr` / `POOL_METRICS_ADDR`, default `:9100`
- Empty string: do not listen

Compose exposes `9100:9100`.

## Alerts (queries only, not provisioned)

- scrape down
- `gopool_chain_head - gopool_last_processed_height > 60` for 5m
- payout failures
- stuck payslips for 15m
- `gopool_validator_state{state="jailed"} == 1`
- payout wallet balance below pending luna
- RPC error spike

## Out of scope

Grafana dashboard JSON, alert provisioning, API RED metrics,
OpenTelemetry, per-staker series, extra `GetValidator` on every tick.
