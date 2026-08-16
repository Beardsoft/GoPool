# Staker self-service: profile, compounding, stuck-payout recovery

## Context

Three related gaps in the staker-facing experience:

1. **Payouts "wait indefinitely".** A payout tx that is broadcast but never
   included in a block stays `awaiting_confirmation` forever. The confirmation
   loop (`internal/pool/confirmation.go`) only finalizes txs the node reports as
   macro-final (`Final && Succeeded` → completed) or macro-final-failed
   (`Final && !Succeeded` → failed). A tx that is *not found* on-chain is treated
   as `ErrNotFound` → skipped → pending indefinitely. On the devnet this is
   guaranteed: the pool validator's on-chain balance is `0` (the devnet simulates
   rewards in the DB but never credits the validator account), so every payout tx
   is mempool-accepted but never block-included. The pool already *counts* these
   as `stuck_payout_count` for health but never *acts* on them.

2. **No per-staker compounding choice.** `payout_mode` is a single global config
   (`delegate` = restake/compound, `transfer` = cash out). A staker cannot choose
   for themselves.

3. **Thin profile.** `MyDashboard` (login via Nimiq Hub already works) shows
   stake / share / rewards / payslips but not the pending-vs-paid split, the
   staker's delegation status, how long a payout has been pending, or a way to
   manage preferences.

## Goals / non-goals

Goals:
- Detect and recover stuck payout txs (epoch-based timeout → failed → retryable)
  + alert the operator.
- Let each staker choose "reinvest as stake" vs "pay me out"; payout logic honors it.
- Enrich the logged-in profile with full staker information + the preference toggle.

Non-goals:
- New-staker onboarding (wallet-signed `CreateStaker`). The initial delegation
  stays a one-time staker-wallet action; out of scope here.
- Funding the devnet validator (a devnet fixture, not product code). Stuck
  detection makes the *consequence* visible and recoverable regardless of cause.

## Design

### 1. Stuck-payout detection + recovery

**Record submission height.** Add `submitted_height INTEGER` to `transactions`,
set at send time in `processApprovedPayouts` (the head is already fetched there).
Migration to add the column.

**Detection.** In `runConfirmations`, for each `awaiting_confirmation` tx whose
`getTransactionByHash` returns not-found: if
`head - submitted_height > stuckPayoutEpochs * blocksPerEpoch` (default
`stuckPayoutEpochs = 3`, new config), the tx is stuck → mark the tx `failed`,
mark its payslips `failed` (existing `UpdatePayslipStatusFailed`), record an
operator event and send a notifier alert of type `payout_stuck`.

**Recovery — operator-only, no auto-retry.** Reuse the existing retry path
(`sql/queries.sql:349–368`): an operator action resets a failed tx's payslips to
`pending` and clears the tx so the next `runPayouts` re-sends. Expose an operator
"retry stuck/failed payouts" action in the operations UI. We deliberately do NOT
auto-retry: a tx that is "not found" may in fact have landed out-of-band, and
auto-retry risks paying a staker twice (the pool already has a known
double-payout hazard on the sign/send-failure path). Operator confirmation is the
safe default; the alert tells them when to act.

**Config.** `stuck_payout_epochs` (default 3). Three epochs is well past
macro-finality (~1–2 epochs), so a healthy network never false-positives.

### 2. Per-staker compounding choice

**Schema.** New table + migration:
```sql
CREATE TABLE IF NOT EXISTS staker_preferences (
    address TEXT PRIMARY KEY,
    compound INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

**Queries.** `GetStakerPreference(address) → compound bool` (no row = use the
global default); `UpsertStakerPreference(address, compound)`.

**API.** Auth required (session):
- `GET /api/me/preference` → `{ "compound": bool }`
- `PUT /api/me/preference` body `{ "compound": bool }` → upsert, returns the stored value.

**Payout logic.** In `runPayouts`, per eligible staker compute the effective mode:
`compound = (row exists ? pref.compound : global payout_mode == "delegate")`.
`stillDelegated` is already computed. `kind = payoutDelegate` when
`compound && stillDelegated`, else `payoutTransfer`. A preference row overrides the
global default; absence falls back to it (backward compatible).

### 3. Full staker profile

**`/api/me` extension.** Add to the response:
- `pending_luna` — sum of payslips in `pending` / `out_for_payment` / `awaiting_confirmation`.
- `paid_luna` — sum of `completed` payslips.
- `delegated` — bool, computed live via one `getStakerByAddress` RPC
  (staker's current delegation == pool validator).
- `preference` — `{ "compound": bool }`.

**MyDashboard UI.**
- Stat row: stake, pool share, cumulative rewards, **pending** (NIM, "waiting to
  pay out"), **paid** (NIM).
- Delegation status line: "Delegated to GoPool" / "Not delegated — rewards not
  accruing".
- Payout table: existing columns + an elapsed-time indicator on pending rows
  ("submitted 2h ago, awaiting confirmation").
- **Compounding toggle**: "Reinvest my rewards as stake" (on) / "Pay me out in
  NIM" (off), wired to `PUT /api/me/preference`.
- Empty / loading / error states consistent with the rest of the app.

**Public Find-My-Stake CTA.** On `StakerLookup`, a "Log in to manage" button that
routes to the dashboard / triggers `loginWithHub`.

## Data flow (payout, with preference + stuck)

1. `runPayouts`: per eligible staker, read preference → choose kind
   (delegate/transfer) → insert `approved` audit log.
2. `processApprovedPayouts`: sign + send → mark payslips `out_for_payment` → on
   success set payslips `awaiting_confirmation`, insert `transactions` row with
   `submitted_height = head`.
3. `runConfirmations`: per `awaiting_confirmation` tx → `getTransactionByHash`:
   - macro-final succeeded → tx `completed`, payslips `completed`.
   - macro-final failed → tx `failed`, payslips `failed`, alert.
   - not-found and `head - submitted_height > 3*blocksPerEpoch` → **stuck**:
     tx `failed`, payslips `failed`, `payout_stuck` event + alert.
4. Operator retry: failed tx's payslips → `pending`, tx cleared → re-sent on the
   next cycle.

## Testing

- **Go**: unit tests for stuck-detection (a tx past the threshold + not-found →
  failed + event), the preference fallback (no row → global default; row present
  → overrides), and `choosePayoutTx` with a per-staker preference. Reuse the
  existing `seedStakerWithPayslip` helper.
- **Web**: Vitest for the profile (pending/paid split, delegation status, toggle
  PUT) and the Find-My-Stake CTA.
- **Devnet**: after deploy, verify a stuck tx flips to failed after 3 epochs and
  is retryable; verify toggling the preference changes the next payout kind.

## Decisions

- Stuck threshold: **epoch-based** (3 epochs), not time-based.
- Phasing: **all three, bug first** — (1) stuck-payout fix, (2) per-staker
  compounding, (3) profile enrichment.
- Retry: **operator-only** (no auto-retry) to avoid double-pay risk.
- `delegated`: computed **live** (one cheap RPC) on `/api/me`, not cached.
