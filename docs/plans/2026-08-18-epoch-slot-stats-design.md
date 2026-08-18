# Epoch Election and Slot Stats Design

## Goal

The operator overview must show whether this validator is in the **current
epoch’s active set**, and how many of that epoch’s slots it holds. “Active” on
the validator account (not retired / inactive / jailed) is a different fact and
stays on the validator card.

## Why

`validator_summary.state === "active"` is account state. A registered, eligible
validator can still have **zero slots** this epoch. The overview currently
cannot tell those apart.

## Contract

- Surface: operator overview metrics strip only.
- Two tiles, not one:
  - **This epoch** — `Elected` / `Not elected`; subtitle `Epoch N`.
  - **Slots** — `12`; subtitle `of 512`.
- Layout: two rows of three. Order:
  1. This epoch
  2. Slots
  3. Chain lag
  4. Delegated stake
  5. Rewards processed
  6. Wallet runway
- Health / attention: unchanged. Not elected does not open an attention item.
- Public pool page, missed-slot tracking, and next-epoch prediction are out of
  scope.

## Data

Daemon already heartbeats into `runtime_status` and refreshes gauges about every
30s. Extend that tick:

1. `getActiveValidators` — membership is the source of truth for “elected”.
2. `getPolicyConstants.slots` — slot denominator (512 on current networks).
   Read it via `Client.Call`; do not hardcode 512. `nimiq-go`’s `Policy` struct
   does not yet include `slots`.
3. Current epoch from existing policy + chain head.

Slot count uses Hamilton / largest remainder over the **active-set balances**
and `slots`:

- `floor(stake_i * slots / total)` for each validator in the set
- leftover slots go to the highest remainders (address as tie-break)
- if our address is not in the set: elected = false, slot_count = 0
- empty set or total stake 0: not elected, 0 slots

Keep the last successful `(epoch, elected, slot_count, slots_total)` tuple in
memory. On RPC failure, rewrite the previous tuple (do not store zeros that
would render as “not elected”). Until the first success, SQL NULLs → UI `—`.

Persist on `runtime_status` (nullable integers). Overview stays a DB read.

JSON on `GET /api/operator/overview`:

```json
"epoch_participation": {
  "epoch": 2,
  "elected": true,
  "slot_count": 12,
  "slots_total": 512
}
```

All four fields are `null` when no snapshot exists yet.

Optional Prometheus gauges next to the existing validator gauges:
`gopool_epoch_elected`, `gopool_validator_slots`, `gopool_slots_total`.

## UI

Unknown snapshot: value `—`, subtitle `Epoch —` / `of —`.
Elected: `Elected`. Unelected: `Not elected` with `0` / `of 512` once a snapshot
exists.

Narrow viewports already stack the metrics grid; keep that, just start from
three columns at desktop.

## Tests

- Slot math: in set, not in set, remainder distribution, empty set.
- Heartbeat writes NULLs until a snapshot, then persists elected + slots.
- Overview JSON exposes `epoch_participation`; attention/health unchanged.
- Vue: both tiles, em dash when null, elected/slot copy when present.
- Migration adds nullable columns on fresh and legacy DBs.

## Out of scope

- Attention when not elected
- Missed / produced slots
- Next-epoch election forecast
- Public `/` proof panel
- `health_snapshots` history of slot counts
