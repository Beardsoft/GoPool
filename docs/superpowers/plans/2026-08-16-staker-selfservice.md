# Staker Self-Service Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Give stakers a full logged-in profile, a per-staker "reinvest vs. cash out" compounding choice, and fix payouts that get stuck at "Awaiting Confirmation" forever.

**Architecture:** Three work items, built bug-first. (1) Stuck-payout recovery: record the block height at payout-send, then in the confirmation loop mark a not-found tx `failed` once it has aged past N epochs (operator retries via the existing path — no auto-retry, to avoid double-pay). (2) Per-staker compounding: a new `staker_preferences` table + `GET/PUT /api/me/preference`; `runPayouts` lets a stored row override the global `payout_mode`, falling back to it when absent. (3) Full profile: `GET /api/me` gains `pending_luna`, `paid_luna`, `delegated` (live RPC), and `compound`; MyDashboard renders the split, delegation status, elapsed time on pending rows, and the toggle; Find-My-Stake gets a "Log in to manage" CTA.

**Tech Stack:** Go (stdlib `net/http`, `database/sql`/sqlite3, nimiq-go v0.3.x RPC client, zap, prometheus), hand-maintained sqlc-style queries (`sql/queries.sql` + `internal/db/queries.sql.go`, **sqlc is NOT installed — edit both by hand**), embedded SQL migrations (`internal/db/migrations/*.sql`, applied by `db.Migrate`), Vue 3 + TypeScript + Vite + Vitest (happy-dom) frontend.

**Design doc:** `docs/superpowers/specs/2026-08-16-staker-selfservice-design.md`

---

## Testing conventions (read once)

- **Pool package** (`internal/pool`): `chain.Chain.RPC` is a concrete `*rpc.Client` (not an interface), so the full Manager loop is not unit-tested. Test (a) **pure decision functions** (table-driven) and (b) **DB query behavior** against in-memory sqlite via the existing helper `newLifecycleTestQueries(t)` (`internal/pool/confirmation_test.go:135`). Extract every new decision into a pure function so it is testable.
- **API package** (`internal/api`): handlers are tested with `httptest` + `newTestDB(t)` (returns `*db.Queries`) + `a.issueSession(addr)` for auth, exactly like `internal/api/staker_handlers_test.go:82-161`.
- **Frontend** (`web`): Vitest + happy-dom; mock `chart.js/auto`; `useExplorer` needs `loadNetwork()` or `resetExplorerForTests()`; `apiGet` throws a plain object `{status,code,message}`.
- Run: `go test ./...` (root), `cd web && npx vitest run`, `cd web && npx vue-tsc --noEmit && npm run build`.
- Commit after every task. Stage **only** the files you touched (the repo has unrelated uncommitted work).

---

# Phase 1 — Stuck-payout detection + recovery

## Task 1: Add `submitted_height` column (migration + backfill)

**Files:** Create: `internal/db/migrations/003_transactions_submitted_height.sql`

**Step 1: Write the migration**

```sql
ALTER TABLE transactions ADD COLUMN submitted_height INTEGER NOT NULL DEFAULT 0;

-- Backfill legacy rows (pre-migration) with the current chain head so they
-- age from "now" instead of being treated as infinitely old. If the head is
-- not yet recorded (0), rows stay 0 and the detector treats 0 as "unknown"
-- and never auto-fails them.
UPDATE transactions
SET submitted_height = (SELECT chain_head FROM runtime_status WHERE id = 1)
WHERE submitted_height = 0;
```

**Step 2: Verify it applies cleanly (fresh + existing DB)**

Run: `go test ./internal/db/ -run TestMigrate -v`
Expected: PASS (the existing `TestMigrateFreshAndLegacyDatabases` migrates a fresh and a legacy DB; the `ALTER TABLE` + `UPDATE` must not error. On a fresh DB the subquery yields NULL → the `UPDATE` sets nothing, which is fine.)

**Step 3: Commit**
```bash
git add internal/db/migrations/003_transactions_submitted_height.sql
git commit -m "db: add transactions.submitted_height for stuck-payout detection"
```

---

## Task 2: Record + read `submitted_height` in queries

`sqlc` is not installed, so edit **both** `sql/queries.sql` (source of truth) and `internal/db/queries.sql.go` (generated code) by hand to keep them in sync.

**Files:** Modify: `sql/queries.sql`, `internal/db/queries.sql.go`, `internal/pool/confirmation_test.go`

**Step 1: Add a failing test** — append to `internal/pool/confirmation_test.go`:
```go
func TestInsertAndReadSubmittedHeight(t *testing.T) {
	q := newLifecycleTestQueries(t)
	if err := q.InsertTransaction(t.Context(), db.InsertTransactionParams{
		Hash: "h1", Address: "A", Amount: 100, Status: "awaiting_confirmation",
		SubmittedHeight: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	pending, err := q.GetPendingTransactions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].SubmittedHeight != 1000 {
		t.Fatalf("pending = %+v, want one row with submitted_height 1000", pending)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/pool/ -run TestInsertAndReadSubmittedHeight -v`
Expected: FAIL (compile error: `SubmittedHeight` undefined).

**Step 3: Update `sql/queries.sql`**
```sql
-- name: InsertTransaction :exec
INSERT INTO transactions (hash, address, amount, status, submitted_height)
VALUES (?, ?, ?, ?, ?);
```
```sql
-- name: GetPendingTransactions :many
SELECT hash, address, amount, submitted_height FROM transactions WHERE status = 'awaiting_confirmation'
```

**Step 4: Update `internal/db/queries.sql.go`**
- `InsertTransaction` const: add `submitted_height` to the column list and a 5th `?`.
- `InsertTransactionParams` struct: add `SubmittedHeight int64`.
- `InsertTransaction` func: add the new arg to the `db.ExecContext` call (5 args).
- `GetPendingTransactions` const: add `submitted_height` to the SELECT.
- `GetPendingTransactionsRow` struct: add `SubmittedHeight int64`.
- `GetPendingTransactions` func: add `&i.SubmittedHeight` to `rows.Scan`.

**Step 5: Run test to verify it passes**

Run: `go test ./internal/pool/ -run TestInsertAndReadSubmittedHeight -v`
Expected: PASS.

**Step 6: Run the whole Go suite to catch other `InsertTransaction` callers**

Run: `go build ./... && go test ./...`
Expected: PASS. (The one other caller, in `processApprovedPayouts`, is fixed in Task 5.)

**Step 7: Commit**
```bash
git add sql/queries.sql internal/db/queries.sql.go internal/pool/confirmation_test.go
git commit -m "db: read/write transactions.submitted_height"
```

---

## Task 3: Add `StuckPayoutEpochs` config (default 3)

**Files:** Modify: `internal/config/config.go`

**Step 1: Add the field + default**
- In the `Config` struct (near `MinPayoutLuna`, ~line 23):
  ```go
  StuckPayoutEpochs int   `json:"stuck_payout_epochs" mapstructure:"stuck_payout_epochs"`
  ```
- In `LoadDaemon` (after the `if cfg.MetricsAddr == ""` block, ~line 236):
  ```go
  if cfg.StuckPayoutEpochs == 0 {
      cfg.StuckPayoutEpochs = 3
  }
  ```
  Do **not** add it to `Editable`/`ValidateEditable` — it is a daemon-side knob, not an operator-editable setting.

**Step 2: Verify it compiles**

Run: `go build ./...`
Expected: PASS.

**Step 3: Commit**
```bash
git add internal/config/config.go
git commit -m "config: add stuck_payout_epochs (default 3)"
```

---

## Task 4: Pure stuck-detection decision function

**Files:** Create: `internal/pool/stuck.go`, `internal/pool/stuck_test.go`

**Step 1: Write the failing test** — `internal/pool/stuck_test.go`:
```go
package pool

import "testing"

func TestIsStuck(t *testing.T) {
	blocksPerEpoch := uint32(43200)
	cases := []struct {
		name string
		sub  int64
		head uint32
		eps  int
		want bool
	}{
		{"unknown submitted height never auto-fails", 0, 500_000, 3, false},
		{"just submitted", 1000, 1000, 3, false},
		{"within threshold", 1000, 100_000, 3, false},
		{"exactly at threshold not stuck", 1000, 130_600, 3, false},
		{"past threshold is stuck", 1000, 130_601, 3, true},
		{"one epoch threshold", 1000, 44_201, 1, true},
	}
	for _, c := range cases {
		if got := isStuck(c.sub, c.head, c.eps, blocksPerEpoch); got != c.want {
			t.Errorf("%s: isStuck = %v, want %v", c.name, got, c.want)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/pool/ -run TestIsStuck -v`
Expected: FAIL (`isStuck` undefined).

**Step 3: Implement** — `internal/pool/stuck.go`:
```go
package pool

// isStuck reports whether a payout transaction that is still not found on-chain
// has aged past the configured number of whole epochs. A non-positive submitted
// height is treated as "unknown" and never auto-fails, so a tx whose height we
// could not record is never marked failed by mistake.
func isStuck(submittedHeight int64, head uint32, stuckEpochs int, blocksPerEpoch uint32) bool {
	if submittedHeight <= 0 || stuckEpochs <= 0 || blocksPerEpoch == 0 {
		return false
	}
	age := int64(head) - submittedHeight
	threshold := int64(stuckEpochs) * int64(blocksPerEpoch)
	return age > threshold
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/pool/ -run TestIsStuck -v`
Expected: PASS.

**Step 5: Commit**
```bash
git add internal/pool/stuck.go internal/pool/stuck_test.go
git commit -m "pool: add isStuck decision for not-found payouts"
```

---

## Task 5: Wire stuck detection into `runConfirmations`

**Files:** Modify: `internal/pool/confirmation.go`, `internal/pool/payout.go`

**Step 1: Record `submitted_height` when a payout is sent**

In `internal/pool/payout.go`, `processApprovedPayouts` already fetches `head` (line 201) and calls `InsertTransaction` (line 266). Add `SubmittedHeight: int64(head)` to the `db.InsertTransactionParams{...}` literal:
```go
_ = m.queries.InsertTransaction(ctx, db.InsertTransactionParams{
	Hash:            hash,
	Address:         log.Address,
	Amount:          log.Amount,
	Status:          "awaiting_confirmation",
	SubmittedHeight: int64(head),
})
```

**Step 2: Add stuck detection to `runConfirmations`**

In `internal/pool/confirmation.go`:
- Fetch the head once at the top of `runConfirmations` (after `GetPendingTransactions`):
  ```go
  head, err := m.chain.RPC.BlockNumber(ctx)
  if err != nil {
      return err
  }
  ```
- In the `ErrNotFound` branch (currently lines 55-57), replace the bare `continue` with a stuck check:
  ```go
  if errors.Is(err, rpc.ErrNotFound) {
      if isStuck(tx.SubmittedHeight, head, m.cfg.StuckPayoutEpochs, m.policy.BlocksPerEpoch) {
          if failErr := m.failStuckPayout(ctx, tx.Hash, tx.Address); failErr != nil {
              return failErr
          }
      }
      continue
  }
  ```
- Add the helper (same file):
  ```go
  // failStuckPayout marks a not-found payout transaction and its payslips as
  // failed and raises a single operator alert. Recovery is operator-driven via
  // the existing retry path; we never auto-resubmit (double-pay risk).
  func (m *Manager) failStuckPayout(ctx context.Context, hash, address string) error {
      if err := m.queries.SetTransactionStatus(ctx, db.SetTransactionStatusParams{Status: "failed", Hash: hash}); err != nil {
          return err
      }
      if err := m.queries.UpdatePayslipStatusFailed(ctx, sql.NullString{String: hash, Valid: true}); err != nil {
          return err
      }
      logger.Logger.Warn("payout stuck, marked failed", zap.String("hash", hash), zap.String("address", address))
      metrics.PayoutsFailed.Inc()
      if m.recorder != nil {
          _ = m.recorder.RecordEvent(ctx, ops.EventInput{
              Severity: "error", Category: "payout", Source: "daemon",
              Type: "payout_stuck", Summary: "Payout stuck (never confirmed), marked failed",
              Context: map[string]any{"txHash": hash, "address": address},
          })
      }
      if m.notifier != nil {
          m.notifier.Send(ctx, notifier.Alert{
              Level: "error", Type: "payout_stuck", Title: "Payout stuck",
              Message: fmt.Sprintf("Payout %s to %s was never confirmed on-chain and has been marked failed. Retry it from the operator console.", hash, address),
          })
      }
      return nil
  }
  ```
- Add imports to `confirmation.go`: `"fmt"`, `"github.com/Beardsoft/GoPool/internal/notifier"`.

**Step 3: Verify it compiles and the suite passes**

Run: `go build ./... && go test ./...`
Expected: PASS.

**Step 4: Commit**
```bash
git add internal/pool/confirmation.go internal/pool/payout.go
git commit -m "pool: fail not-found payouts stuck past N epochs + alert"
```

---

## Task 6: Verify operator retry recovers a stuck payout

The existing `RetryFailedPayoutPayslips` query resets `failed` payslips tied to a `failed` tx back to `pending`. Confirm it works for a stuck→failed payout with a query-level test.

**Files:** Test: `internal/pool/confirmation_test.go`

**Step 1: Write the test** — append to `internal/pool/confirmation_test.go`:
```go
func TestStuckFailedPayoutCanBeRetried(t *testing.T) {
	q := newLifecycleTestQueries(t)
	ctx := t.Context()
	if err := q.InsertPayslip(ctx, db.InsertPayslipParams{
		BatchNumber: 1, EpochNumber: 1, Address: "A", Amount: 1000, Status: "pending",
	}); err != nil {
		t.Fatal(err)
	}
	if err := q.SetPayslipsTransaction(ctx, db.SetPayslipsTransactionParams{
		TxHash: sql.NullString{String: "h1", Valid: true}, Address: "A",
	}); err != nil {
		t.Fatal(err)
	}
	if err := q.SetTransactionStatus(ctx, db.SetTransactionStatusParams{Status: "failed", Hash: "h1"}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdatePayslipStatusFailed(ctx, sql.NullString{String: "h1", Valid: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RetryFailedPayoutPayslips(ctx, "h1"); err != nil {
		t.Fatal(err)
	}
	payslips, err := q.GetPayslipsForAddress(ctx, "A")
	if err != nil {
		t.Fatal(err)
	}
	if len(payslips) != 1 || payslips[0].Status != "pending" || payslips[0].TxHash.Valid {
		t.Fatalf("after retry payslips = %+v, want pending with cleared tx", payslips)
	}
}
```
> Note: confirm the exact `InsertPayslip` param field names against `internal/db/queries.sql.go` before running; adjust to match (columns are `batch_number, epoch_number, address, amount, status`).

**Step 2: Run test**

Run: `go test ./internal/pool/ -run TestStuckFailedPayoutCanBeRetried -v`
Expected: PASS.

**Step 3: Commit**
```bash
git add internal/pool/confirmation_test.go
git commit -m "test: stuck-failed payout is recoverable via operator retry"
```

---

# Phase 2 — Per-staker compounding choice

## Task 7: Add `staker_preferences` table (migration)

**Files:** Create: `internal/db/migrations/004_staker_preferences.sql`

**Step 1: Write the migration**
```sql
CREATE TABLE IF NOT EXISTS staker_preferences (
    address TEXT PRIMARY KEY,
    compound INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```
(`compound = 1` → reinvest as stake; `compound = 0` → pay out. A missing row means "use the global payout_mode".)

**Step 2: Verify it applies**

Run: `go test ./internal/db/ -run TestMigrate -v`
Expected: PASS.

**Step 3: Commit**
```bash
git add internal/db/migrations/004_staker_preferences.sql
git commit -m "db: add staker_preferences table for per-staker compounding"
```

---

## Task 8: `GetStakerPreference` / `UpsertStakerPreference` queries

**Files:** Modify: `sql/queries.sql`, `internal/db/queries.sql.go`, `internal/pool/confirmation_test.go`

**Step 1: Write the failing test** — append to `internal/pool/confirmation_test.go`:
```go
func TestStakerPreferenceUpsertAndGet(t *testing.T) {
	q := newLifecycleTestQueries(t)
	ctx := t.Context()

	if _, err := q.GetStakerPreference(ctx, "A"); err != sql.ErrNoRows {
		t.Fatalf("absent preference err = %v, want sql.ErrNoRows", err)
	}

	if err := q.UpsertStakerPreference(ctx, db.UpsertStakerPreferenceParams{Address: "A", Compound: 1}); err != nil {
		t.Fatal(err)
	}
	pref, err := q.GetStakerPreference(ctx, "A")
	if err != nil || pref != 1 {
		t.Fatalf("after upsert pref = %d, err = %v, want 1", pref, err)
	}

	if err := q.UpsertStakerPreference(ctx, db.UpsertStakerPreferenceParams{Address: "A", Compound: 0}); err != nil {
		t.Fatal(err)
	}
	pref, err = q.GetStakerPreference(ctx, "A")
	if err != nil || pref != 0 {
		t.Fatalf("after overwrite pref = %d, err = %v, want 0", pref, err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/pool/ -run TestStakerPreferenceUpsertAndGet -v`
Expected: FAIL (`GetStakerPreference` / `UpsertStakerPreference` undefined).

**Step 3: Add the SQL** to `sql/queries.sql`:
```sql
-- name: GetStakerPreference :one
SELECT compound FROM staker_preferences WHERE address = ?;

-- name: UpsertStakerPreference :exec
INSERT INTO staker_preferences (address, compound, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(address) DO UPDATE SET compound = excluded.compound, updated_at = CURRENT_TIMESTAMP;
```

**Step 4: Add the generated code** to `internal/db/queries.sql.go`:
- `GetStakerPreference` const + `func (q *Queries) GetStakerPreference(ctx, address string) (int64, error)` — scan a single `int64` (return `sql.ErrNoRows` when no row, matching the `:one` convention used elsewhere).
- `UpsertStakerPreferenceParams` struct `{ Address string; Compound int64 }` + `const` + `func (q *Queries) UpsertStakerPreference(ctx, p UpsertStakerPreferenceParams) error`.
- Mirror the exact scan/exec boilerplate of a neighbouring `:one` and `:exec` query in the file.

**Step 5: Run test to verify it passes**

Run: `go test ./internal/pool/ -run TestStakerPreferenceUpsertAndGet -v`
Expected: PASS.

**Step 6: Commit**
```bash
git add sql/queries.sql internal/db/queries.sql.go internal/pool/confirmation_test.go
git commit -m "db: get/upsert staker_preferences"
```

---

## Task 9: Pure effective-kind decision

**Files:** Create: `internal/pool/compounding.go`, `internal/pool/compounding_test.go`

The decision: given the global `payout_mode`, an optional stored preference (`*bool`, nil = absent), and whether the staker is still delegated, which tx kind do we send? A stored row overrides the global mode; an absent row falls back to it. `delegate` still requires `stillDelegated` (reuse `choosePayoutTx`).

**Step 1: Write the failing test** — `internal/pool/compounding_test.go`:
```go
package pool

import "testing"

func boolPtr(b bool) *bool { return &b }

func TestEffectivePayoutKind(t *testing.T) {
	cases := []struct {
		name string
		mode string
		pref *bool
		dep  bool // stillDelegated
		want payoutKind
	}{
		{"global delegate, no pref, delegated", "delegate", nil, true, payoutDelegate},
		{"global delegate, no pref, undelegated", "delegate", nil, false, payoutTransfer},
		{"global transfer, no pref", "transfer", nil, true, payoutTransfer},
		{"pref compound beats global transfer", "transfer", boolPtr(true), true, payoutDelegate},
		{"pref compound but undelegated falls back", "transfer", boolPtr(true), false, payoutTransfer},
		{"pref cashout beats global delegate", "delegate", boolPtr(false), true, payoutTransfer},
	}
	for _, c := range cases {
		if got := effectivePayoutKind(c.mode, c.pref, c.dep); got != c.want {
			t.Errorf("%s: effectivePayoutKind = %v, want %v", c.name, got, c.want)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/pool/ -run TestEffectivePayoutKind -v`
Expected: FAIL (`effectivePayoutKind` undefined).

**Step 3: Implement** — `internal/pool/compounding.go`:
```go
package pool

// effectivePayoutKind resolves the payout tx kind for one staker. A stored
// preference (pref != nil) overrides the global mode; a nil pref falls back to
// the global mode. "delegate" only restakes while the staker is still delegated
// to this validator (choosePayoutTx), otherwise it degrades to a transfer.
func effectivePayoutKind(globalMode string, pref *bool, stillDelegated bool) payoutKind {
	mode := globalMode
	if pref != nil {
		if *pref {
			mode = "delegate"
		} else {
			mode = "transfer"
		}
	}
	return choosePayoutTx(mode, stillDelegated)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/pool/ -run TestEffectivePayoutKind -v`
Expected: PASS.

**Step 5: Commit**
```bash
git add internal/pool/compounding.go internal/pool/compounding_test.go
git commit -m "pool: add effectivePayoutKind (per-staker preference override)"
```

---

## Task 10: Honor the per-staker preference in `runPayouts`

**Files:** Modify: `internal/pool/payout.go`

**Step 1: Replace the kind selection in `runPayouts`**

In `internal/pool/payout.go`, `runPayouts` currently computes `kind` inline (lines 109-114):
```go
		kind := payoutTransfer
		if m.cfg.PayoutMode == "delegate" {
			staker, err := m.chain.RPC.GetStaker(ctx, addr)
			stillDelegated := err == nil && staker.Delegation == m.chain.Address().String()
			kind = choosePayoutTx(m.cfg.PayoutMode, stillDelegated)
		}
```
Replace it so the stored preference is consulted first, and the `GetStaker` RPC is only made when the resolved mode is `delegate`:
```go
		pref, perr := m.queries.GetStakerPreference(ctx, row.Address)
		var prefPtr *bool
		if perr == nil {
			b := pref == 1
			prefPtr = &b
		}

		resolvedMode := m.cfg.PayoutMode
		if prefPtr != nil && *prefPtr {
			resolvedMode = "delegate"
		} else if prefPtr != nil {
			resolvedMode = "transfer"
		}

		kind := payoutTransfer
		if resolvedMode == "delegate" {
			staker, err := m.chain.RPC.GetStaker(ctx, addr)
			stillDelegated := err == nil && staker.Delegation == m.chain.Address().String()
			kind = effectivePayoutTx(resolvedMode, prefPtr, stillDelegated)
		}
```
> Note: `effectivePayoutTx` (below) wraps `effectivePayoutKind` so the resolved-mode + stillDelegated are passed cleanly. If you prefer, call `effectivePayoutKind(resolvedMode, prefPtr, stillDelegated)` directly — the two are equivalent; keep one and delete the other.

Add the tiny wrapper (or skip it and call `effectivePayoutKind` directly):
```go
func effectivePayoutTx(mode string, pref *bool, stillDelegated bool) payoutKind {
	return effectivePayoutKind(mode, pref, stillDelegated)
}
```

**Step 2: Verify it compiles and the suite passes**

Run: `go build ./... && go test ./...`
Expected: PASS.

**Step 3: Commit**
```bash
git add internal/pool/payout.go
git commit -m "pool: runPayouts honors per-staker compounding preference"
```

---

## Task 11: `GET/PUT /api/me/preference`

**Files:** Modify: `internal/api/staker_handlers.go` (register routes + handlers), `internal/api/staker_handlers_test.go`

**Step 1: Write the failing test** — append to `internal/api/staker_handlers_test.go`:
```go
func TestMePreferencePutAndGet(t *testing.T) {
	q := newTestDB(t)
	a := &API{queries: q, cfg: &config.Config{SessionSecret: "test-secret"}}
	addr, _ := nimiq.ParseAddress(testStakerAddress)
	a.Mux()

	// PUT compound=true
	body := `{"compound":true}`
	req := httptest.NewRequest(http.MethodPut, "/api/me/preference", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: a.issueSession(addr)})
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// GET returns the stored choice
	req2 := httptest.NewRequest(http.MethodGet, "/api/me/preference", nil)
	req2.AddCookie(&http.Cookie{Name: sessionCookieName, Value: a.issueSession(addr)})
	rec2 := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", rec2.Code, rec2.Body.String())
	}
	if !strings.Contains(rec2.Body.String(), `"compound":true`) {
		t.Fatalf("GET body = %s, want compound:true", rec2.Body.String())
	}

	// Unauthenticated GET is 401
	rec3 := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/api/me/preference", nil))
	if rec3.Code != http.StatusUnauthorized {
		t.Fatalf("unauth GET status = %d, want 401", rec3.Code)
	}
}
```
> Note: reuse the existing `testStakerAddress` constant and imports (`net/http`, `net/http/httptest`, `strings`, `nimiq`) already present in `staker_handlers_test.go`; add `strings` if missing.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestMePreferencePutAndGet -v`
Expected: FAIL (no route → 404/405).

**Step 3: Register routes + add handlers** in `internal/api/staker_handlers.go`

In `registerStakerRoutes` (line 95-102) add:
```go
	mux.HandleFunc("GET /api/me/preference", a.requireSession(a.handleGetMyPreference))
	mux.HandleFunc("PUT /api/me/preference", a.requireSession(a.handlePutMyPreference))
```
Add the handlers:
```go
// effectiveCompound returns the stored preference, or the value implied by the
// global payout_mode when the staker has not chosen yet.
func (a *API) effectiveCompound(ctx context.Context, address string) (bool, error) {
	pref, err := a.queries.GetStakerPreference(ctx, address)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return a.cfg.PayoutMode == "delegate", nil
		}
		return false, err
	}
	return pref == 1, nil
}

func (a *API) handleGetMyPreference(w http.ResponseWriter, r *http.Request) {
	addr, ok := addressFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	compound, err := a.effectiveCompound(r.Context(), addr.String())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading preference")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"compound": compound})
}

func (a *API) handlePutMyPreference(w http.ResponseWriter, r *http.Request) {
	addr, ok := addressFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	var body struct {
		Compound bool `json:"compound"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := a.queries.UpsertStakerPreference(r.Context(), db.UpsertStakerPreferenceParams{
		Address: addr.String(), Compound: boolToInt(body.Compound),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "saving preference")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"compound": body.Compound})
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
```
Add imports to `staker_handlers.go` if missing: `"database/sql"`, `"encoding/json"`, and `db` (already imported).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestMePreferencePutAndGet -v`
Expected: PASS.

**Step 5: Run the full Go suite**

Run: `go build ./... && go test ./...`
Expected: PASS.

**Step 6: Commit**
```bash
git add internal/api/staker_handlers.go internal/api/staker_handlers_test.go
git commit -m "api: GET/PUT /api/me/preference for per-staker compounding"
```

---

# Phase 3 — Full logged-in staker profile

## Task 12: Extend `GET /api/me` with profile fields

**Files:** Modify: `internal/api/staker_handlers.go`, `internal/api/staker_handlers_test.go`

`GET /api/me` currently returns `stakerDetailResponse` (`address, stake_luna, percentage, payslips, transactions`). Add `pending_luna`, `paid_luna`, `delegated`, and `compound`. Keep the public `GET /api/stakers/{address}` unchanged (no live RPC there).

**Step 1: Write the failing test** — append to `internal/api/staker_handlers_test.go`:
```go
func TestMeProfileFields(t *testing.T) {
	q := newTestDB(t)
	a := &API{queries: q, cfg: &config.Config{SessionSecret: "test-secret", PayoutMode: "delegate"}}
	addr, _ := nimiq.ParseAddress(testStakerAddress)

	// Seed a staker + one completed and one pending payslip.
	ctx := t.Context()
	must := func(err error) { if err != nil { t.Fatal(err) } }
	must(q.InsertStaker(ctx, db.InsertStakerParams{EpochNumber: 1, Address: addr.String(), Stake: 1000000, Percentage: 0.5}))
	must(q.InsertPayslip(ctx, db.InsertPayslipParams{BatchNumber: 1, EpochNumber: 1, Address: addr.String(), Amount: 500, Status: "completed"}))
	must(q.InsertPayslip(ctx, db.InsertPayslipParams{BatchNumber: 2, EpochNumber: 2, Address: addr.String(), Amount: 700, Status: "pending"}))

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: a.issueSession(addr)})
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/me status = %d, body = %s", rec.Code, rec.Body.String())
	}
	b := rec.Body.String()
	for _, want := range []string{`"pending_luna":700`, `"paid_luna":500`, `"delegated":`, `"compound":`} {
		if !strings.Contains(b, want) {
			t.Fatalf("body missing %s; got %s", want, b)
		}
	}
}
```
> Note: confirm `InsertStaker` / `InsertPayslip` param field names against `internal/db/queries.sql.go` before running; adjust to the real column set. `delegated` will be `false` here because `a.rpc` is nil (guarded).

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestMeProfileFields -v`
Expected: FAIL (missing fields).

**Step 3: Add the response type + logic** in `internal/api/staker_handlers.go`

Add near `stakerDetailResponse`:
```go
type meResponse struct {
	stakerDetailResponse
	PendingLuna int64 `json:"pending_luna"`
	PaidLuna    int64 `json:"paid_luna"`
	Delegated   bool  `json:"delegated"`
	Compound    bool  `json:"compound"`
}
```
Change `handleMe` to build and return `meResponse` (replace the final `writeJSON(w, http.StatusOK, detail)`):
```go
	detail, err := stakerDetail(r.Context(), a.queries, addr)
	if errors.Is(err, errStakerNotFound) {
		writeError(w, http.StatusNotFound, "this address has never staked with this pool")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading staker")
		return
	}

	var pending, paid int64
	for _, p := range detail.Payslips {
		switch p.Status {
		case "completed":
			paid += p.AmountLuna
		case "pending", "out_for_payment", "awaiting_confirmation":
			pending += p.AmountLuna
		}
	}

	delegated := false
	if a.rpc != nil && a.cfg != nil && a.cfg.ValidatorAddress != "" {
		if staker, rerr := a.rpc.GetStaker(r.Context(), addr); rerr == nil {
			delegated = staker.Delegation == a.cfg.ValidatorAddress
		}
	}
	compound, cerr := a.effectiveCompound(r.Context(), addr.String())
	if cerr != nil {
		writeError(w, http.StatusInternalServerError, "loading preference")
		return
	}

	writeJSON(w, http.StatusOK, meResponse{
		stakerDetailResponse: detail,
		PendingLuna:          pending,
		PaidLuna:             paid,
		Delegated:            delegated,
		Compound:             compound,
	})
```
> `effectiveCompound` is added in Task 11; if you implement Phase 3 before Task 11, add it here. `payslipResponse.Status` and `.AmountLuna` already exist (see `stakerDetail`).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestMeProfileFields -v`
Expected: PASS.

**Step 5: Run the full Go suite**

Run: `go build ./... && go test ./...`
Expected: PASS.

**Step 6: Commit**
```bash
git add internal/api/staker_handlers.go internal/api/staker_handlers_test.go
git commit -m "api: enrich GET /api/me with pending/paid/delegated/compound"
```

---

## Task 13: MyDashboard profile UI

**Files:** Modify: `web/src/pages/MyDashboard.vue` (and its test if present: `web/src/pages/MyDashboard.test.ts`)

Wire the new `/api/me` fields into the dashboard:
- **Pending vs. paid split:** two stat cards — "Awaiting payout" (`pending_luna` via `formatNim`) and "Paid out" (`paid_luna`).
- **Delegation status:** a badge — "Delegated to pool" (green, `delegated === true`) or "Not delegated" (neutral). When not delegated, show a hint that payouts will be sent as transfers, not restaked.
- **Elapsed time on pending rows:** in the payouts table, for rows with status `pending`/`out_for_payment`/`awaiting_confirmation`, show "waiting Xh Ym" computed from `submitted_at`/`created_at` (add a small `elapsed(ts)` helper; reuse `formatNim` for amounts).
- **Compounding toggle:** a switch bound to `compound` from `/api/me`. On change, `PUT /api/me/preference` with `{compound: value}`; on success update local state, on failure show an inline error and revert. Labels: "Reinvest as stake" (on) / "Pay me out" (off).

Implementation notes:
- The dashboard already fetches `/api/me` (see existing `onMounted`). Read the new fields from the same response — no new fetch needed.
- Use the existing `apiPost`/`apiPut`-style helper for the PUT; if only `apiGet`/`apiPost` exist, add a tiny `apiPut(url, body)` wrapper mirroring `apiPost` (same `{status,code,message}` error shape).
- Reuse `NimAmount.vue`, `StatusBadge.vue`, and the design tokens; do not introduce new colors.
- Keep the existing payouts table; only add the elapsed column and the two stat cards + toggle.

**Step 1: Update/extend the component** (follow the existing structure in `MyDashboard.vue`).

**Step 2: Add/extend the Vitest test** — assert the pending/paid cards render `formatNim` values, the delegation badge text, and that toggling fires a `PUT /api/me/preference` with the right body (mock the API layer).

**Step 3: Run the frontend checks**

Run: `cd web && npx vitest run && npx vue-tsc --noEmit && npm run build`
Expected: PASS (all tests, clean type-check, successful build).

**Step 4: Commit**
```bash
git add web/src/pages/MyDashboard.vue web/src/pages/MyDashboard.test.ts
git commit -m "web: MyDashboard profile (pending/paid, delegation, elapsed, compounding toggle)"
```

---

## Task 14: Find-My-Stake "Log in to manage" CTA

**Files:** Modify: `web/src/pages/StakerLookup.vue` (and `web/src/pages/StakerLookup.test.ts`)

On the public Find-My-Stake page, add a call-to-action that invites the staker to log in for the full profile + controls:
- A button/link "Log in to manage your stake" that routes to the dashboard (`/dashboard` or whatever `MyDashboard`'s route is — confirm via `web/src/router`), preserving the looked-up address if the app supports a `?address=` deep link; otherwise just link to the login/dashboard.
- Place it near the top (under the address identity) so it is visible without scrolling.
- Reuse the existing button styles + `--nimiq-light-blue` accent; no new dependencies.

**Step 1: Add the CTA** to `StakerLookup.vue`.

**Step 2: Extend the Vitest test** — assert the CTA element exists and points to the dashboard route.

**Step 3: Run the frontend checks**

Run: `cd web && npx vitest run && npx vue-tsc --noEmit && npm run build`
Expected: PASS.

**Step 4: Commit**
```bash
git add web/src/pages/StakerLookup.vue web/src/pages/StakerLookup.test.ts
git commit -m "web: Find-My-Stake CTA to log in and manage"
```

---

# Final verification

Run the full gate before declaring done:
```bash
go build ./... && go test ./...
cd web && npx vitest run && npx vue-tsc --noEmit && npm run build
```
Expected: all Go tests pass, all web tests pass, clean type-check, successful build.

Then deploy to devnet (`make dev-api-rebuild`) and verify live:
1. **Stuck payouts:** the 100 stuck txs should, after 3 devnet epochs of not-found, flip to `failed` and raise a `payout_stuck` alert; confirm via the operator console + an alert delivery.
2. **Compounding:** as a logged-in staker, toggle "Reinvest as stake" and confirm `GET /api/me/preference` returns the stored value and a subsequent payout uses the chosen kind.
3. **Profile:** confirm `/api/me` returns `pending_luna`, `paid_luna`, `delegated`, `compound` and MyDashboard renders them.

---

## Execution handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-16-staker-selfservice.md`. Two execution options:

1. **Subagent-Driven (this session)** — dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Parallel Session (separate)** — open a new session with `executing-plans`, batch execution with checkpoints.
