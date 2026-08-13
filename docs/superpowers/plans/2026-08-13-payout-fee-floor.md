# Payout Fee Floor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Hold payouts until the amount is at least 10× the estimated tx fee, pay that fee from the pool, and stop hardcoding fee `0`.

**Architecture:** Keep `min_payout_luna` as the SQL eligibility gate. For each eligible staker, build the unsigned payout tx, `EstimateFee`, and skip (leave `pending`) when `amount < fee * 10`. On send, set `tx.Fee` to the estimate. Reset `out_for_payment` → `pending` if submit fails after the mark.

**Tech Stack:** Go, existing `internal/pool` + `nimiq-go` (`rpc.Client.EstimateFee`), sqlc/SQLite.

**Spec:** `docs/superpowers/specs/2026-08-13-payout-fee-floor-design.md`

---

### Task 1: `payoutWorthSending` helper

**Files:**
- Modify: `internal/pool/payout.go`
- Modify: `internal/pool/payout_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/pool/payout_test.go`:

```go
func TestPayoutWorthSending(t *testing.T) {
	cases := []struct {
		amount, fee nimiq.Luna
		want        bool
	}{
		{1, 0, true},        // zero-fee network: min_payout already applied in SQL
		{99, 10, false},     // 10×10 = 100, hold
		{100, 10, true},     // exactly 10×
		{9, 1, false},
		{10, 1, true},
	}
	for _, c := range cases {
		if got := payoutWorthSending(c.amount, c.fee); got != c.want {
			t.Errorf("payoutWorthSending(%d, %d) = %v, want %v", c.amount, c.fee, got, c.want)
		}
	}
}
```

Add `"github.com/NimMiniApps/nimiq-go"` to the imports (keep `"testing"`).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/pool/ -run TestPayoutWorthSending -count=1`

Expected: FAIL — `payoutWorthSending` undefined

- [ ] **Step 3: Minimal implementation**

Add to `internal/pool/payout.go` (below the `payoutKind` consts):

```go
// feePayoutMultiple is how many times the tx fee the pending amount must
// cover before we send. The pool pays the fee on top; this keeps that cost
// from eating the payout.
const feePayoutMultiple = 10

// payoutWorthSending reports whether amount is large enough to justify
// paying fee. A zero fee (current Nimiq default) always passes — the SQL
// min_payout_luna gate has already run.
func payoutWorthSending(amount, fee nimiq.Luna) bool {
	if fee == 0 {
		return true
	}
	return amount >= fee*feePayoutMultiple
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/pool/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/pool/payout.go internal/pool/payout_test.go
git commit -m "$(cat <<'EOF'
feat(pool): hold payouts until amount covers 10× tx fee

EOF
)"
```

---

### Task 2: Reset `out_for_payment` payslips by address

**Files:**
- Modify: `sql/queries.sql`
- Modify: `internal/db/queries.sql.go` (via `sqlc generate`, or hand-write to match sqlc style if `sqlc` is missing)

Needed so a failed submit after `MarkPayslipsOutForPayment` does not stick slips forever. `ResetPayslipsToPending` keys on `tx_hash`, which does not exist yet on submit failure.

- [ ] **Step 1: Add the query**

Append to `sql/queries.sql`:

```sql
-- name: ResetPayslipsOutForPayment :exec
UPDATE payslips SET status = 'pending' WHERE address = ? AND status = 'out_for_payment';
```

- [ ] **Step 2: Generate (or hand-write) the Go method**

Run: `sqlc generate`

If `sqlc` is not installed, add this next to `ResetPayslipsToPending` in `internal/db/queries.sql.go` (sqlc would place it alphabetically before `ResetPayslipsToPending`):

```go
const ResetPayslipsOutForPayment = `-- name: ResetPayslipsOutForPayment :exec
UPDATE payslips SET status = 'pending' WHERE address = ? AND status = 'out_for_payment'
`

func (q *Queries) ResetPayslipsOutForPayment(ctx context.Context, address string) error {
	_, err := q.db.ExecContext(ctx, ResetPayslipsOutForPayment, address)
	return err
}
```

- [ ] **Step 3: Compile**

Run: `go build ./internal/db/`

Expected: PASS (no output)

- [ ] **Step 4: Commit**

```bash
git add sql/queries.sql internal/db/queries.sql.go
git commit -m "$(cat <<'EOF'
feat(db): reset out-for-payment payslips by address

EOF
)"
```

---

### Task 3: Estimate fee, hold, and submit with a real fee

**Files:**
- Modify: `internal/pool/payout.go`

- [ ] **Step 1: Extract unsigned tx construction**

Replace `submitPayout` with:

```go
func (m *Manager) buildPayoutTx(recipient nimiq.Address, amount, fee nimiq.Luna, kind payoutKind, head uint32) (*nimiq.Transaction, error) {
	sender := m.chain.Address()
	if kind == payoutDelegate {
		return nimiq.NewAddStakeTransaction(sender, recipient, amount, fee, head, m.chain.Network)
	}
	return nimiq.NewBasicTransaction(sender, recipient, amount, fee, head, m.chain.Network)
}

func (m *Manager) signAndSend(ctx context.Context, tx *nimiq.Transaction, kind payoutKind) (string, error) {
	if kind == payoutDelegate {
		if err := nimiq.SignStakingTransaction(ctx, tx, m.chain.Signer, m.chain.Signer); err != nil {
			return "", err
		}
	} else if err := nimiq.SignTransaction(ctx, m.chain.Signer, tx); err != nil {
		return "", err
	}
	return m.chain.RPC.SendTransaction(ctx, tx)
}
```

- [ ] **Step 2: Wire the loop — estimate and skip before marking**

Replace the body of the `for _, row := range eligible` loop in `runPayouts` with:

```go
		addr, err := nimiq.ParseAddress(row.Address)
		if err != nil {
			logger.Logger.Error("unparseable staker address, skipping", zap.String("address", row.Address), zap.Error(err))
			continue
		}
		amount := nimiq.Luna(uint64(row.Total))

		kind := payoutTransfer
		if m.cfg.PayoutMode == "delegate" {
			staker, err := m.chain.RPC.GetStaker(ctx, addr)
			stillDelegated := err == nil && staker.Delegation == m.chain.Address().String()
			kind = choosePayoutTx(m.cfg.PayoutMode, stillDelegated)
		}

		tx, err := m.buildPayoutTx(addr, amount, 0, kind, head)
		if err != nil {
			logger.Logger.Error("building payout tx", zap.String("address", row.Address), zap.Error(err))
			continue
		}
		fee, err := m.chain.RPC.EstimateFee(ctx, tx)
		if err != nil {
			logger.Logger.Error("estimating payout fee", zap.String("address", row.Address), zap.Error(err))
			continue
		}
		if !payoutWorthSending(amount, fee) {
			logger.Logger.Debug("holding payout until amount covers 10× fee",
				zap.String("staker", row.Address),
				zap.Uint64("amount", uint64(amount)),
				zap.Uint64("fee", uint64(fee)))
			continue
		}
		tx.Fee = fee

		if err := m.queries.MarkPayslipsOutForPayment(ctx, row.Address); err != nil {
			return err
		}

		hash, err := m.signAndSend(ctx, tx, kind)
		if err != nil {
			if resetErr := m.queries.ResetPayslipsOutForPayment(ctx, row.Address); resetErr != nil {
				logger.Logger.Error("resetting payslips after failed submit", zap.String("address", row.Address), zap.Error(resetErr))
			}
			logger.Logger.Error("payout submission failed, will retry next tick", zap.String("address", row.Address), zap.Error(err))
			continue
		}

		if err := m.queries.SetPayslipsTransaction(ctx, db.SetPayslipsTransactionParams{
			TxHash:  sql.NullString{String: hash, Valid: true},
			Address: row.Address,
		}); err != nil {
			return err
		}
		if err := m.queries.InsertTransaction(ctx, db.InsertTransactionParams{
			Hash: hash, Address: row.Address, Amount: int64(amount), Status: "awaiting_confirmation",
		}); err != nil {
			return err
		}
		logger.Logger.Info("payout submitted", zap.String("staker", row.Address), zap.Uint64("amount", uint64(amount)), zap.String("tx", hash))
```

Leave the rest of `runPayouts` (eligible fetch, `BlockNumber`, return) unchanged.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/pool/ -count=1`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/pool/payout.go
git commit -m "$(cat <<'EOF'
feat(pool): estimate payout fees and hold uneconomical sends

EOF
)"
```

---

## Out of scope (do not do)

- Protocol `minimum_stake` checks on payouts
- New config fields
- Reward-address vs payout-wallet split
- Validator deactivate failures
