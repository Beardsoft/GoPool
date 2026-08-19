# Validator Autostake Bootstrap Design

## Goal

A freshly installed pool becomes an on-chain validator without the operator
manually funding, registering, or self-staking. After setup the daemon waits
for funds, registers the validator, then stakes leftover NIM to itself so the
account can be elected.

## Why

Today `ensureReady` retries until `getValidatorByAddress` succeeds. The README
tells the operator to stake or register by hand. The separate
`nimiq-validator-activator` already does testnet faucet + `sendNewValidatorTransaction`
once 100k NIM (the protocol validator deposit) is on the address. Folding that
into GoPool removes the extra service and the leftover manual CreateStaker.

`nimiq-go` cannot sign `CreateValidator` (BLS proof of knowledge). Registration
therefore uses the bundled validator node's wallet RPC, which already has
`[rpc-server]` on port 8648 and the signing/voting keys from onboarding
`wallet.json`.

## Contract

- Testnet (`test-albatross`): POST the validator address to
  `https://faucet.pos.nimiq-testnet.com/tapit` until liquid balance reaches
  **101,000 NIM**.
- Mainnet / others: no faucet. Wait until the operator sends funds.
- When liquid NIM covers the validator deposit, register via the **local
  validator node's wallet RPC only** (`importRawKey`, `unlockAccount`,
  `sendNewValidatorTransaction`). Keys come from onboarding `wallet.json`.
- After the validator exists, `CreateStaker` the leftover (minus a 10 NIM fee
  reserve) delegated to self, signed offline with the existing pool key and
  broadcast on the pool RPC.
- Pool reads and ordinary broadcasts stay on configured `rpc_url`. The local
  validator RPC is a **fallback** (`WithFallbackEndpoints`) for those calls,
  and the **only** endpoint for wallet/CreateValidator.
- Do not publish validator RPC to the host or public internet.
- Missing `wallet.json` or unreachable local RPC: skip auto-register, keep
  waiting as today. Do not fail the daemon.

## RPC split

| Call | Endpoint |
| --- | --- |
| Policy, balance, GetValidator, GetStaker, CreateStaker, payouts, reactivate | `rpc_url`, fallback `validator_rpc_url` on transport/5xx |
| `isConsensusEstablished`, `importRawKey`, `unlockAccount`, `sendNewValidatorTransaction` | `validator_rpc_url` only |

Compose/Swarm already put `gopool` and `gopool-validator` on the same network.
`client.toml` already binds RPC to `0.0.0.0:8648`. Default
`validator_rpc_url` is `http://gopool-validator:8648`. No host port mapping.

## Bootstrap sequence

Runs inside the existing `waitReady` loop (not after it). Each pass:

1. Load policy (`ValidatorDeposit`, `MinimumStake`). Deposit is 100,000 NIM on
   current networks; minimum stake is 100 NIM.
2. Read liquid balance of the pool wallet.
3. If `network == test-albatross` and a faucet URL is set and
   `balance < 101_000 NIM`, POST `address=<validator>` to the faucet. Cooldown
   10s. Ignore non-OK; retry next pass.
4. If `GetValidator` is not found, `wallet.json` is present, local node has
   consensus, no pending register action, and
   `balance >= ValidatorDeposit`: `sendNewValidatorTransaction`. Cooldown 60s.
   Record `validator_actions` (`create`) like reactivate.
5. If the validator exists, `GetStaker(pool)` is not found, and
   `balance - 10 NIM >= MinimumStake`: `CreateStaker` leftover minus reserve,
   delegated to self. Record `validator_actions` (`self_stake`).
6. If the validator exists, `ensureReady` continues with today's wallet
   identity checks and the main loop starts.

Do not `AddStake` leftover later. One self-stake at bootstrap. Later NIM on
the address is operator/delegator money and payout runway.

## Keys

Onboarding already writes `.secrets/wallet.json` (address, signing, fee, BLS
voting). The daemon already has the address key via `POOL_PRIVATE_KEY_FILE`.

Mount `wallet.json` read-only as `POOL_WALLET_JSON_FILE`. The daemon reads
`signing_private_key` and `voting_secret_key` from it. The API never sees this
file.

If the file is absent, log once and skip CreateValidator. After a confirmed
register the operator may delete `wallet.json` from the server (backup first);
self-stake and the rest of the pool do not need it.

## Config

Not operator-editable (not on Settings). Deployment env / optional config
fields:

- `faucet_url` — empty default; Compose testnet sets the tapit URL. Ignored
  unless `network == test-albatross`.
- `validator_rpc_url` — empty default; Compose/Swarm set
  `http://gopool-validator:8648`.
- `POOL_WALLET_JSON_FILE` — path to `wallet.json`.

No new `auto_register` checkbox. Presence of wallet JSON + validator RPC
enables register; faucet URL enables testnet funding.

## Operator UI

Reuse heartbeat `readiness_error` plus daemon events (`category=validator`,
info). Summaries:

- Waiting for funds to register the validator (include current vs deposit).
- Requested testnet faucet funds.
- Registration submitted (tx hash).
- Self-stake submitted (tx hash).

Setup launch poll already shows `readiness_error`. Replace the generic
“validator not found” with the current bootstrap sentence when that event
exists. Overview attention lists those events like other validator items.
No new overview tiles.

## Errors

- Faucet HTTP failure: log, retry after 10s. Never crash.
- Local node no consensus / RPC down: skip CreateValidator this pass.
- CreateValidator / CreateStaker broadcast failure: record failed action,
  60s cooldown, retry.
- Pending action row blocks a second submit (same pattern as
  `runAutoReactivate`).
- Dry-run: skip faucet, register, and self-stake.

## Tests

- Pure decision table: faucet / register / self-stake / wait, covering
  mainnet (no faucet), missing wallet JSON, pending action, balance just
  below deposit, validator exists but no staker, already staked.
- Faucet client: form POST, non-OK, timeout (httptest).
- Wallet RPC client: import/unlock/createValidator against a fake node;
  never pointed at pool RPC in tests.
- `waitReady` integration: missing validator → faucet stub funds → register
  stub → CreateStaker → `ensureReady` succeeds.
- Frontend: setup/overview show the bootstrap sentence instead of a raw
  “validator not found”.

## Out of scope

- BLS / `CreateValidator` in `nimiq-go`.
- `UpdateValidator`.
- Publishing validator RPC to the host.
- Devnet genesis-key faucet (`devlab/create_test_stakers`).
- Auto `AddStake` after the first self-stake.
- Activator sidecar.
