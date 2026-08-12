# GoPool API + Frontend Design

## Goal

Make the pool usable by people other than someone reading SQLite directly:
stakers can look up their stake and payout history, and the operator can
check daemon health and trigger validator lifecycle actions (deactivate,
retire) without shelling into the host and running CLI subcommands.

This adds two new components. It does not change the daemon's core
election/checkpoint/payout logic (Tasks 1-9 of
`docs/superpowers/plans/2026-08-12-validator-pool-daemon.md`), only extends
its main loop with one more per-tick check (see "Operator action flow"
below).

## Non-goals

- No wallet balance display, staking UI (delegate/undelegate), or anything
  that constructs transactions on behalf of a staker. Stakers already stake
  through their own wallet; this is read visibility into the pool plus
  operator controls, not a wallet.
- No multi-pool / multi-tenant support. One daemon, one validator, one API
  instance, matching the daemon's existing single-validator assumption.
- No historical analytics beyond what's already stored (epochs, rewards,
  payslips, transactions, validator_actions). No new aggregation tables.

## Architecture

```
cmd/                  existing daemon — unchanged, sole holder of the
                       validator private key, sole DB writer for pool state
cmd/api/               new: API server entrypoint
internal/api/           new: HTTP handlers, routing, Hub-auth middleware
web/                     new: Vue 3 + Nimiq UI Kit SPA (Vite), embedded via
                         go:embed into cmd/api
```

`cmd/api` is a separate binary/process from the daemon, sharing the same
SQLite file. It opens the DB read-only for all data endpoints. The one
exception is inserting `validator_actions` rows with `status='requested'`
for operator-triggered deactivate/retire (see below) — the API never signs
or submits a transaction itself.

SQLite in WAL mode supports concurrent readers alongside the daemon's
writes without extra coordination; no new locking is needed.

## Auth: Nimiq Hub, shared by stakers and operator

1. `POST /api/auth/challenge` → server generates a random nonce, stores it
   server-side with a short TTL (e.g. 5 minutes), returns it.
2. Client signs the nonce via Nimiq Hub's sign-message flow.
3. `POST /api/auth/verify` with the signature → server recovers the address
   from the signature, checks it matches the nonce and hasn't expired,
   issues a signed session cookie scoped to that address. The nonce is
   consumed (single use).
4. If the recovered address equals the configured operator/validator
   address (from the daemon's config), the session is additionally
   operator-scoped. Every other address gets a staker session scoped only
   to itself.

No separate operator secret to provision or rotate — operator-ness is
"controls the validator's private key," the same identity the daemon itself
already uses.

Session cookies: signed (HMAC, server-held key from config), short-lived
(e.g. 24h), httpOnly, `SameSite=Lax`.

## Endpoints

Public, no auth required — everything here is already publicly derivable
from chain RPC (`getStakerByAddress`, `getValidatorByAddress`), so gating it
behind login would only add friction, not privacy:

- `GET /api/pool` — current epoch number, epoch status, total delegated
  stake, num stakers, cumulative rewards paid, configured fee %
- `GET /api/epochs` — list of epochs with status
- `GET /api/epochs/{n}` — epoch detail: stakers and their percentages
- `GET /api/stakers/{address}` — current stake %, payslip history, linked
  transaction statuses

Session-gated, staker or operator — convenience wrapper resolving the
caller's own address instead of requiring them to type it:

- `GET /api/me` — same shape as `GET /api/stakers/{address}` for the
  session's address

Operator-only (session must be operator-scoped, else 403):

- `GET /api/operator/health` — daemon cursor height vs current chain head
  (lag), count and list of stuck payslips (status `awaiting_confirmation`
  or `out_for_payment` past some age threshold)
- `POST /api/operator/validator/deactivate` — inserts a `validator_actions`
  row (`action='deactivate'`, `status='requested'`), returns 202
- `POST /api/operator/validator/retire` — same, `action='retire'`

## Operator action flow

The daemon's `Run()` loop (`internal/pool/pool.go`) gains one more per-tick
check, alongside its existing election/checkpoint/payout/confirmation
steps: look for `validator_actions` rows with `status='requested'`, call
the existing `Deactivate()` / `Retire()` methods (`internal/pool/lifecycle.go`,
unchanged), and update the row to `submitted` (with tx hash) or `failed`.

This keeps the validator private key in exactly one process. The API
never signs or submits a transaction — it only ever writes a request row
and reads status back.

## Data model changes

`validator_actions` already has `action`, `attempted_at`, `tx_hash`,
`outcome` (`internal/db` / `schema/scheme.sql`). Add:

- `status TEXT NOT NULL DEFAULT 'requested'` — `requested` → `submitted` /
  `failed`, mirroring the payslip status lifecycle already in use.

No other schema changes; all other endpoints read existing tables
(`epochs`, `stakers`, `rewards`, `payslips`, `transactions`).

## Frontend

Vue 3 + Nimiq UI Kit, built with Vite, `go:embed`ed into the `cmd/api`
binary — one artifact to deploy alongside the daemon binary, no separate
static host.

Pages:
- Pool overview (`/`) — current epoch, stake, rewards, fee, public
- Epoch list / detail (`/epochs`, `/epochs/:n`) — public
- Staker lookup (`/stakers/:address`) — public, just paste an address
- My dashboard (`/me`) — Hub login, resolves to the caller's own address
- Operator panel (`/operator`) — Hub login, operator-scoped only: daemon
  health, stuck payslips, deactivate/retire buttons

## Error handling

- Unknown address in `GET /api/stakers/{address}` → 404, not an empty
  200, so the frontend can distinguish "never staked here" from "staked
  with zero balance."
- Expired/invalid session cookie → 401 on session-gated routes, prompting
  re-login rather than silently falling back to public data.
- Non-operator session on `/api/operator/*` → 403.
- Nonce reuse or expiry on `/api/auth/verify` → 400, client requests a
  fresh challenge.

## Testing

Table-driven handler tests per endpoint using `net/http/httptest` against a
seeded in-memory SQLite, matching the existing `internal/pool` test style
(`internal/pool/*_test.go`). Minimum coverage:

- Each public endpoint against seeded data, including the 404 case for an
  unknown staker address.
- Auth flow: valid challenge/verify round-trip produces a session; expired
  or reused nonce is rejected.
- Authorization: operator-address session reaches `/api/operator/*`;
  staker-address session gets 403.
- Operator action flow: `POST /api/operator/validator/deactivate` inserts
  the expected `validator_actions` row; a daemon-side test confirms the
  new poll step picks up a `requested` row and calls `Deactivate()`.
