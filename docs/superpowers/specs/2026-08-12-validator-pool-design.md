# GoPool: full-fledged validator pool daemon

## Context

GoPool tracks delegators to a Nimiq validator, computes each delegator's
share of validator rewards per epoch, and pays them out. The algorithm is
extracted from `zpool` (a working Zig implementation) and re-expressed on top
of `nimiq-go` as the SDK, instead of the current hand-rolled JSON-RPC client
in `internal/rpc`.

This spec assumes the `nimiq-go` SDK additions in
[`2026-08-12-pool-sdk-additions-design.md`](../../../../nimiq-go/docs/superpowers/specs/2026-08-12-pool-sdk-additions-design.md)
are implemented first: `GetStakersByValidatorAddress`,
`GetInherentsByBlockNumber`, and the `Deactivate`/`Reactivate`/`Retire`/
`DeleteValidator` transaction builders. GoPool cannot be built without them.

The pool assumes a validator already exists and is running — GoPool does not
create validators (needs BLS, out of scope per the SDK spec). It manages an
existing validator's active/jailed/retired lifecycle and handles reward
distribution.

## Architecture

One goroutine, ticker-driven (~2s). Each tick:

1. Read current height via `rpc.Client.BlockNumber`.
2. Read `cursor.last_processed_height` from the DB.
3. Replay every height between cursor and current height in order,
   classifying each as **micro**, **checkpoint**
   (`height % BatchSize == 0`), or **election**
   (`height % EpochSize == 0`) using `GetPolicy`'s constants.
4. Advance the cursor after each height is handled.

This replaces zpool's separate poller-thread/worker-thread/in-memory-queue
split with one loop — the DB cursor already gives crash-safe resume, and pool
throughput doesn't need two threads. No "collection" grouping of batches
either (zpool batches N checkpoints together to cut RPC calls); each
checkpoint is processed directly. Add batching back only if
`GetInherentsByBlockNumber` call volume is measured to be a real cost.

## DB schema (replaces the current sqlc schema)

- `cursor(name PK, height)` — single row, `last_processed_height`.
- `policy_constants` — kept as-is, still cached at startup.
- `epochs(number PK, num_stakers, balance, status, created_at)`.
- `stakers(epoch_number, address, stake, percentage)` — snapshot taken at
  election.
- `rewards(epoch_number, batch_number, amount, pool_fee, num_stakers, created_at)`.
- `payslips(id, batch_number, address, amount, status, tx_hash)`.
- `transactions(hash PK, address, amount, status, submitted_at)`.
- `validator_actions(id, action, attempted_at, tx_hash, outcome)` — audit
  log and idempotency guard for automated lifecycle transactions.

Status is a Go enum, not a lookup table — a fixed small set of constants
doesn't need normalizing into SQL:
- epoch: `NotElected`, `NoStakers`, `Retired`, `Inactive`, `Jailed`,
  `InProgress`, `Completed`.
- payout: `Pending`, `OutForPayment`, `AwaitingConfirmation`, `Completed`,
  `Failed`.

All Luna amounts use `nimiq.Luna` (already `uint64`-backed in the SDK), not
`float64` — the current code's use of `float64` for on-chain amounts is a
precision bug on a money path and must not carry over.

## Core flows

**Election** (last micro block before an election block, or the election
block itself): `GetValidator` + `GetStakersByValidatorAddress`, compute each
staker's percentage of total delegated balance, snapshot into `stakers` for
the new epoch. If the validator is inactive/retired/jailed/has zero stakers,
record the epoch with that status and skip reward work for it — mirrors
zpool's `fetchValidatorDetails`.

**Checkpoint**: `GetInherentsByBlockNumber(height - BatchSize)`, filter to
`IsReward()` inherents addressed to the pool's validator, deduct the
configured pool fee, insert one `rewards` row, fan out `payslips` rows per
staker using the epoch's stored percentages.

**Payout**: every tick, sum each staker's pending payslips; once a staker
crosses the configured minimum payout (in Luna), build a payout transaction
and mark those payslips `OutForPayment`. Payout mode is configurable
pool-wide, defaulting to delegate:
- **Delegate (default)**: `GetStaker` to confirm the staker is still
  delegated to this validator; if so, `NewAddStakeTransaction` (compounds
  their stake); if they've undelegated, fall back to a plain
  `NewBasicTransaction`. This fixes a known TODO in zpool, which always
  restakes without checking.
- **Transfer**: always `NewBasicTransaction`.

**Confirmation tracking**: poll pending `transactions` each tick; once a
macro block has passed since submission and execution succeeded, mark
`Completed` and finalize the linked payslips; on failure, reset payslips to
`Pending` for retry.

**Epoch finalization**: once every payslip tied to an epoch's rewards is
`Completed`, mark the epoch `Completed`.

## Validator lifecycle

Conservative default — these are real staking transactions with consequences
for delegators:

- **Auto-reactivate**: if the validator is jailed and the current height has
  passed `jailedFrom` + cooldown, send `NewReactivateValidatorTransaction`
  once, logged to `validator_actions` and guarded so it isn't resent every
  tick (check for an existing unconfirmed `validator_actions` row for the
  same action before sending another).
- **Deactivate / retire / delete**: exposed as CLI subcommands
  (`gopool validator deactivate|retire|delete`) run deliberately by the
  operator — not automated, since they can pull the pool out of the active
  set or affect delegator funds. Not: `gopool validator create` — validator
  creation stays outside GoPool (see Context).

## Config additions

- `payout_mode`: `"delegate"` (default) | `"transfer"`.
- `min_payout_luna`: existing.
- `pool_fee_percentage`: existing.
- `auto_reactivate`: bool, default `true`.

## Phased delivery

1. (In `nimiq-go`, separate spec) SDK additions.
2. Schema + sqlc queries.
3. Daemon main loop — election/checkpoint/payout/confirmation/finalization,
   built on the `nimiq-go` SDK; delete `internal/rpc`'s hand-rolled JSON
   client in favor of `rpc.Client`.
4. Validator lifecycle: auto-reactivate + CLI subcommands.
5. Config/docs/devnet end-to-end pass.

## Non-goals

- Validator creation (needs BLS — see `nimiq-go` spec).
- Collection-style batching of checkpoints (zpool has it; not carried over,
  see Architecture).
