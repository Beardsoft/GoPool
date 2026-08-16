# One-Wallet Payout Safety Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make one pool wallet safely receive rewards and sign payouts, while preventing unfunded or duplicate payout attempts and recording exact submission heights.

**Architecture:** Keep the existing daemon-only signer as the sole wallet. Validate it against both configured validator identity and the validator's live reward address before entering the loop. Gate payout audit creation and broadcast against a reserved in-memory balance budget, and persist the head used to build each transaction.

**Tech Stack:** Go, nimiq-go v0.4.0, SQLite/sqlc, Bash, Docker Compose, core-rs-albatross devnet.

---

### Task 1: Enforce the one-wallet startup contract

**Files:**
- Modify: `internal/pool/pool.go`
- Test: `internal/pool/pool_test.go`

**Step 1: Write the failing tests**

Add table-driven tests around a small `validatePoolWallet(configured, derived string, validator *rpc.Validator) error` helper. Cover a configured-validator mismatch, an on-chain validator-address mismatch, a reward-address mismatch, and the valid all-equal case.

**Step 2: Verify RED**

Run: `go test ./internal/pool -run TestValidatePoolWallet -count=1`

Expected: build failure because `validatePoolWallet` does not exist.

**Step 3: Implement minimal validation**

Normalize addresses with `nimiq.ParseAddress`, return descriptive errors containing the expected and actual addresses, and fetch the validator in `Manager.Run` before starting the ticker. Return the readiness error instead of silently returning `nil`.

**Step 4: Verify GREEN**

Run: `go test ./internal/pool -run 'TestValidatePoolWallet|Test.*Validator' -count=1`

Expected: PASS.

### Task 2: Hold payouts when the wallet cannot cover them

**Files:**
- Modify: `internal/pool/payout.go`
- Modify: `internal/pool/pool.go`
- Test: `internal/pool/payout_test.go`

**Step 1: Write failing tests**

Test `payoutFitsBalance(balance, amount, fee)` at exact balance, one luna short, and overflow boundaries. Test the generic hold-state helper so the first insufficient-balance tick alerts, repeated ticks do not, and recovery permits a future alert.

**Step 2: Verify RED**

Run: `go test ./internal/pool -run 'TestPayoutFitsBalance|TestBalanceHold' -count=1`

Expected: build failure because the balance helpers do not exist.

**Step 3: Implement minimal balance budgeting**

Read the pool wallet balance once in `runPayouts`, reserve `amount + fee` for each audit created, and leave obligations pending when the remaining balance is insufficient. Repeat the balance check in `processApprovedPayouts` for pre-existing approved audit rows, decrementing the local budget only after successful broadcast. Emit a deduplicated `insufficient_payout_balance` alert and do not mutate payslips for held rows.

**Step 4: Verify GREEN**

Run: `go test ./internal/pool -run 'TestPayoutFitsBalance|TestBalanceHold|TestPayoutWorthSending' -count=1`

Expected: PASS.

### Task 3: Prevent duplicate active attempts

**Files:**
- Modify: `sql/queries.sql`
- Regenerate: `internal/db/queries.sql.go`
- Test: `internal/pool/confirmation_test.go`

**Step 1: Write the failing database test**

Seed pending payslips plus either an approved payout audit or an awaiting-confirmation transaction for the same address. Assert `GetEligibleForPayout` excludes that address while still returning an unrelated eligible address.

**Step 2: Verify RED**

Run: `go test ./internal/pool -run TestEligiblePayoutExcludesActiveAttempt -count=1`

Expected: FAIL because the address is currently returned.

**Step 3: Implement and regenerate**

Add `NOT EXISTS` guards for approved payout audit rows and awaiting-confirmation transactions to `GetEligibleForPayout`. Run `/home/maestro/go/bin/sqlc generate`.

**Step 4: Verify GREEN**

Run: `go test ./internal/pool -run TestEligiblePayoutExcludesActiveAttempt -count=1`

Expected: PASS.

### Task 4: Persist the real submission height

**Files:**
- Modify: `internal/pool/payout.go`
- Test: `internal/pool/payout_test.go`

**Step 1: Write the failing test**

Test a small `newPayoutTransaction(hash, address string, amount int64, head uint32)` constructor and require its `SubmittedHeight` to equal `head`.

**Step 2: Verify RED**

Run: `go test ./internal/pool -run TestNewPayoutTransactionRecordsHead -count=1`

Expected: build failure because the constructor does not exist.

**Step 3: Implement and use it**

Return `db.InsertTransactionParams` with all fields populated, including `SubmittedHeight: int64(head)`, and use it after successful broadcast.

**Step 4: Verify GREEN**

Run: `go test ./internal/pool -run 'TestNewPayoutTransactionRecordsHead|TestInsertAndReadSubmittedHeight' -count=1`

Expected: PASS.

### Task 5: Make the devnet use the one wallet

**Files:**
- Modify: `devlab/albatross/genesis/src/genesis/dev-albatross-4-validators.toml`
- Test: `devlab/prepare_test.go`

**Step 1: Write the failing fixture test**

Parse or inspect the first validator fixture and assert its `validator_address` and `reward_address` are both `NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E`.

**Step 2: Verify RED**

Run: `go test ./devlab -run TestDevnetFirstValidatorUsesOneWallet -count=1`

Expected: FAIL because the reward address is currently `NQ46...813A`.

**Step 3: Correct the fixture**

Change only the first validator's reward address and update its comment. No second secret or mount is added.

**Step 4: Verify GREEN**

Run: `go test ./devlab -run 'TestDevnetFirstValidatorUsesOneWallet|TestDevnetValidatorKeyMatchesConfiguredValidator' -count=1`

Expected: PASS.

### Task 6: Verify and perform recoverable devnet acceptance

**Files:**
- Verify only: all modified source and existing user changes
- Runtime backup: timestamped sibling of `data/pool.db`

**Step 1: Run static gates**

Run: `gofmt` on owned Go edits, `git diff --check`, `go test ./...`, `go vet ./...`, frontend tests/build, `bash -n devlab/prepare.sh`, and Docker Compose config validation.

**Step 2: Preserve current diagnostics**

Stop the devnet, validate that `data/pool.db` is a regular non-symlink file within the repository, and move it to a timestamped backup under `data/backups/`. Do not delete it.

**Step 3: Rebuild and exercise**

Rebuild the Albatross image so the corrected genesis is embedded, start GoPool, seed stakers if needed, and wait for a reward plus payout confirmation.

**Step 4: Prove acceptance**

Query RPC and SQLite to prove the live validator reward address equals the signer, the pool wallet received rewards, a payout has non-zero `submitted_height`, the transaction is macro-final and successful, and its payslip is completed. Confirm logs contain no repeating absent-hash storm.
