# Validator Pool Daemon Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace GoPool's hand-rolled JSON-RPC pool logic with a daemon built on `nimiq-go`: track delegators, collect rewards, pay out stakers, track confirmations, and automate safe validator-lifecycle recovery.

**Architecture:** One goroutine, ticker-driven, replaying block heights between a DB cursor and the current head, classifying each as election/checkpoint/micro via `nimiq-go`'s `rpc.Policy`. Election snapshots staker percentages; checkpoint fetches reward inherents and fans out payslips; a payout pass batches payslips per staker once a minimum is reached; a confirmation pass settles pending transactions and finalizes epochs.

**Tech Stack:** Go 1.21, SQLite via `mattn/go-sqlite3` + `sqlc` (already in use), `github.com/NimMiniApps/nimiq-go` and its `rpc`/`signer` subpackages (new dependency), `spf13/viper` for config (already in use).

## Global Constraints

- **Depends on the nimiq-go plan** (`docs/superpowers/plans/2026-08-12-pool-sdk-additions.md` in the `nimiq-go` repo): `rpc.Client.GetStakersByValidatorAddress`, `rpc.Client.GetInherentsByBlockNumber`, `nimiq.NewReactivateValidatorTransaction`, `nimiq.NewDeactivateValidatorTransaction`, `nimiq.NewRetireValidatorTransaction`, `nimiq.NewDeleteValidatorTransaction` must exist and be released (or replaced with a `go.mod replace` to a local checkout) before Task 3 onward.
- All on-chain amounts are `nimiq.Luna` end to end — no `float64` on any money path. Store as SQLite `INTEGER` (Luna fits in `int64`; `MaxLuna` is `2^53-1`).
- Status columns are free-text matching the Go string constants defined in Task 1 — no numeric status table, the set is fixed and small.
- No `float64` for stake percentages either where it can be avoided in comparisons, but `percentage REAL` for the *stored snapshot* is fine — it is a ratio for display/estimation, not a payout amount; payout amounts are computed from integer stake ratios directly (`stake * reward / totalStake`) in Task 4, not from the stored `REAL`.
- Every table gets a matching sqlc query file under `sql/`; do not write raw SQL inline in Go — this repo's existing convention is 100% sqlc-generated queries.
- No new CLI framework dependency (no cobra) — subcommand dispatch is a plain `os.Args[1]` switch, matching this repo's otherwise-minimal dependency list.

---

## File Structure

- Modify `schema/scheme.sql` — full schema rewrite (drops the old `payouts`/`pool_payouts`/`last_processed_checkpoint` shape).
- Modify `sql/queries.sql` — full query rewrite.
- Regenerate `internal/db/*.go` via `sqlc generate` (do not hand-edit generated files).
- Modify `internal/config/config.go` — add payout mode, min payout, auto-reactivate, network fields; drop API-key/RPC-import fields the SDK makes unnecessary.
- Delete `internal/rpc/` — replaced entirely by `nimiq-go`'s `rpc` package.
- Delete `internal/models/` — `Validator`/`Inherent`/`PolicyConstants` are now `nimiq-go`'s own types.
- Delete `internal/helper/` if nothing outside `internal/rpc` used it (checked in Task 2).
- Create `internal/chain/client.go` — builds the `rpc.Client` + `signer.PrivateKey` from config; the one place that knows how to talk to the node.
- Rewrite `internal/pool/pool.go` — the daemon's main loop and height classification only.
- Create `internal/pool/election.go` — election-block handling.
- Create `internal/pool/checkpoint.go` — checkpoint-block reward collection.
- Create `internal/pool/payout.go` — payout batching (delegate/transfer modes).
- Create `internal/pool/confirmation.go` — transaction confirmation + epoch finalization.
- Create `internal/pool/lifecycle.go` — auto-reactivate + the four CLI-driven lifecycle actions.
- Rewrite `cmd/main.go` — daemon entrypoint plus `validator` subcommand dispatch.

---

## Task 1: Schema and query rewrite

**Files:**
- Modify: `schema/scheme.sql`
- Modify: `sql/queries.sql`
- Delete: nothing yet (old Go files are deleted in Task 2, once nothing references the old generated code)

**Interfaces:**
- Consumes: nothing.
- Produces: sqlc-generated `db.Queries` methods and `db.*Params` structs used by every later task:
  `GetCursor`, `UpsertCursor`, `InsertPolicyConstants`, `GetLatestPolicyConstants`,
  `EpochExists`, `InsertEpoch`, `GetEpochStatus`, `SetEpochStatus`, `FinalizeCompletedEpochs`,
  `InsertStaker`, `GetStakersForEpoch`,
  `RewardExists`, `InsertReward`,
  `InsertPayslip`, `GetEligibleForPayout`, `MarkPayslipsOutForPayment`, `SetPayslipsTransaction`, `FinalizePayslips`, `ResetPayslipsToPending`,
  `InsertTransaction`, `GetPendingTransactions`, `SetTransactionStatus`,
  `InsertValidatorAction`, `HasPendingValidatorAction`.

- [ ] **Step 1: Write the new schema**

Replace the full contents of `schema/scheme.sql`:

```sql
-- schema.sql
CREATE TABLE IF NOT EXISTS cursor (
    name TEXT PRIMARY KEY,
    height INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS policy_constants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    staking_contract_address TEXT NOT NULL,
    transaction_validity_window INTEGER NOT NULL,
    blocks_per_batch INTEGER NOT NULL,
    batches_per_epoch INTEGER NOT NULL,
    blocks_per_epoch INTEGER NOT NULL,
    validator_deposit INTEGER NOT NULL,
    minimum_stake INTEGER NOT NULL,
    total_supply INTEGER NOT NULL,
    block_separation_time INTEGER NOT NULL,
    jail_epochs INTEGER NOT NULL,
    genesis_block_number INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS epochs (
    number INTEGER PRIMARY KEY,
    num_stakers INTEGER NOT NULL,
    balance INTEGER NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS stakers (
    epoch_number INTEGER NOT NULL,
    address TEXT NOT NULL,
    stake INTEGER NOT NULL,
    percentage REAL NOT NULL,
    PRIMARY KEY (epoch_number, address),
    FOREIGN KEY (epoch_number) REFERENCES epochs(number)
);

CREATE TABLE IF NOT EXISTS rewards (
    batch_number INTEGER PRIMARY KEY,
    epoch_number INTEGER NOT NULL,
    amount INTEGER NOT NULL,
    pool_fee INTEGER NOT NULL,
    num_stakers INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (epoch_number) REFERENCES epochs(number)
);

CREATE TABLE IF NOT EXISTS payslips (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    batch_number INTEGER NOT NULL,
    address TEXT NOT NULL,
    amount INTEGER NOT NULL,
    status TEXT NOT NULL,
    tx_hash TEXT,
    FOREIGN KEY (batch_number) REFERENCES rewards(batch_number)
);

CREATE TABLE IF NOT EXISTS transactions (
    hash TEXT PRIMARY KEY,
    address TEXT NOT NULL,
    amount INTEGER NOT NULL,
    status TEXT NOT NULL,
    submitted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS validator_actions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    action TEXT NOT NULL,
    attempted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    tx_hash TEXT,
    outcome TEXT NOT NULL
);
```

- [ ] **Step 2: Write the new queries**

Replace the full contents of `sql/queries.sql`:

```sql
-- name: GetCursor :one
SELECT height FROM cursor WHERE name = ?;

-- name: UpsertCursor :exec
INSERT INTO cursor (name, height) VALUES (?, ?)
ON CONFLICT (name) DO UPDATE SET height = excluded.height;

-- name: InsertPolicyConstants :exec
INSERT INTO policy_constants (
    staking_contract_address, transaction_validity_window, blocks_per_batch,
    batches_per_epoch, blocks_per_epoch, validator_deposit, minimum_stake,
    total_supply, block_separation_time, jail_epochs, genesis_block_number
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetLatestPolicyConstants :one
SELECT staking_contract_address, transaction_validity_window, blocks_per_batch,
    batches_per_epoch, blocks_per_epoch, validator_deposit, minimum_stake,
    total_supply, block_separation_time, jail_epochs, genesis_block_number
FROM policy_constants ORDER BY id DESC LIMIT 1;

-- name: EpochExists :one
SELECT EXISTS(SELECT 1 FROM epochs WHERE number = ?);

-- name: InsertEpoch :exec
INSERT INTO epochs (number, num_stakers, balance, status) VALUES (?, ?, ?, ?);

-- name: GetEpochStatus :one
SELECT status, num_stakers FROM epochs WHERE number = ?;

-- name: SetEpochStatus :exec
UPDATE epochs SET status = ? WHERE number = ?;

-- name: FinalizeCompletedEpochs :many
UPDATE epochs SET status = 'completed' WHERE number IN (
    SELECT r.epoch_number
    FROM rewards AS r
    INNER JOIN (
        SELECT batch_number, COUNT(*) AS total, SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS done
        FROM payslips
        GROUP BY batch_number
    ) AS p ON p.batch_number = r.batch_number
    INNER JOIN epochs AS e ON e.number = r.epoch_number
    WHERE e.status = 'in_progress'
    GROUP BY r.epoch_number
    HAVING SUM(p.total) = SUM(p.done)
) RETURNING number;

-- name: InsertStaker :exec
INSERT INTO stakers (epoch_number, address, stake, percentage) VALUES (?, ?, ?, ?);

-- name: GetStakersForEpoch :many
SELECT address, stake, percentage FROM stakers WHERE epoch_number = ?;

-- name: RewardExists :one
SELECT EXISTS(SELECT 1 FROM rewards WHERE batch_number = ?);

-- name: InsertReward :exec
INSERT INTO rewards (batch_number, epoch_number, amount, pool_fee, num_stakers)
VALUES (?, ?, ?, ?, ?);

-- name: InsertPayslip :exec
INSERT INTO payslips (batch_number, address, amount, status) VALUES (?, ?, ?, ?);

-- name: GetEligibleForPayout :many
SELECT address, SUM(amount) AS total
FROM payslips
WHERE status = 'pending'
GROUP BY address
HAVING SUM(amount) >= ?;

-- name: MarkPayslipsOutForPayment :exec
UPDATE payslips SET status = 'out_for_payment' WHERE address = ? AND status = 'pending';

-- name: SetPayslipsTransaction :exec
UPDATE payslips SET tx_hash = ?, status = 'awaiting_confirmation'
WHERE address = ? AND status = 'out_for_payment';

-- name: FinalizePayslips :exec
UPDATE payslips SET status = 'completed' WHERE tx_hash = ?;

-- name: ResetPayslipsToPending :exec
UPDATE payslips SET tx_hash = NULL, status = 'pending' WHERE tx_hash = ?;

-- name: InsertTransaction :exec
INSERT INTO transactions (hash, address, amount, status) VALUES (?, ?, ?, ?);

-- name: GetPendingTransactions :many
SELECT hash, address, amount FROM transactions WHERE status = 'awaiting_confirmation';

-- name: SetTransactionStatus :exec
UPDATE transactions SET status = ? WHERE hash = ?;

-- name: InsertValidatorAction :exec
INSERT INTO validator_actions (action, tx_hash, outcome) VALUES (?, ?, ?);

-- name: HasPendingValidatorAction :one
SELECT EXISTS(SELECT 1 FROM validator_actions WHERE action = ? AND outcome = 'pending');
```

- [ ] **Step 3: Regenerate sqlc output**

Run: `sqlc generate`
Expected: `internal/db/models.go` and `internal/db/queries.sql.go` are rewritten with the methods and params structs listed under Interfaces above. No manual edits to these files.

- [ ] **Step 4: Verify it compiles in isolation**

Run: `go build ./internal/db/...`
Expected: succeeds (the rest of the module will not yet build — `internal/pool` and `internal/rpc` still reference the old schema — that's expected until Task 2 removes them).

- [ ] **Step 5: Commit**

```bash
git add schema/scheme.sql sql/queries.sql internal/db
git commit -m "feat(db): rewrite schema for zpool-style pool tracking"
```

---

## Task 2: Config rewrite and chain client wiring

**Files:**
- Modify: `internal/config/config.go`
- Modify: `config/config.json-example`
- Create: `internal/chain/client.go`
- Delete: `internal/rpc/` (all files)
- Delete: `internal/models/` (all files)
- Delete: `internal/helper/` (only if nothing besides the deleted `internal/rpc` imports it — check with `grep -rl 'internal/helper' --include=*.go .` first)

**Interfaces:**
- Consumes: `nimiq-go`'s `nimiq.NetworkID`, `signer.ParsePrivateKeyHex`, `rpc.New`, `rpc.WithNetwork`.
- Produces:
  - `config.Config` fields: `RPCURL string`, `PoolFeeWallet string`, `PoolFeePercentage float64`, `PrivateKey string`, `Network string`, `PayoutMode string` (`"delegate"` or `"transfer"`), `MinPayoutLuna uint64`, `AutoReactivate bool`.
  - `chain.Chain` struct in the new `internal/chain` package: `type Chain struct { RPC *rpc.Client; Signer *signer.PrivateKey; Network nimiq.NetworkID }`, built by `func New(cfg *config.Config) (*Chain, error)`.

- [ ] **Step 1: Add the nimiq-go dependency**

Run: `go get github.com/NimMiniApps/nimiq-go@latest`
Expected: `go.mod`/`go.sum` updated. If the SDK plan's additions have not been tagged yet, use a `replace` directive in `go.mod` pointing at the local `nimiq-go` checkout instead:
```
replace github.com/NimMiniApps/nimiq-go => ../nimiq-go
```

- [ ] **Step 2: Rewrite config**

Replace `internal/config/config.go`:

```go
package config

import (
	"fmt"

	"github.com/Beardsoft/GoPool/internal/logger"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Config struct {
	RPCURL            string  `mapstructure:"rpc_url"`
	Network           string  `mapstructure:"network"`
	PoolFeeWallet     string  `mapstructure:"pool_fee_wallet"`
	PoolFeePercentage float64 `mapstructure:"pool_fee_percentage"`
	PrivateKey        string  `mapstructure:"private_key"`
	PayoutMode        string  `mapstructure:"payout_mode"`
	MinPayoutLuna     uint64  `mapstructure:"min_payout_luna"`
	AutoReactivate    bool    `mapstructure:"auto_reactivate"`
}

func (c *Config) Validate() error {
	if c.PayoutMode != "delegate" && c.PayoutMode != "transfer" {
		return fmt.Errorf("config: payout_mode must be \"delegate\" or \"transfer\", got %q", c.PayoutMode)
	}
	if c.PoolFeePercentage < 0 || c.PoolFeePercentage >= 1 {
		return fmt.Errorf("config: pool_fee_percentage must be in [0, 1), got %v", c.PoolFeePercentage)
	}
	return nil
}

func LoadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	viper.SetEnvPrefix("POOL")
	viper.SetDefault("payout_mode", "delegate")
	viper.SetDefault("auto_reactivate", true)

	if err := viper.ReadInConfig(); err != nil {
		logger.Logger.Error("Error reading config file", zap.Error(err))
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		logger.Logger.Error("Unable to decode config into struct", zap.Error(err))
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	logger.Logger.Info("Config loaded successfully",
		zap.String("rpc_url", cfg.RPCURL),
		zap.String("payout_mode", cfg.PayoutMode),
		zap.Float64("pool_fee_percentage", cfg.PoolFeePercentage))

	return &cfg, nil
}
```

Update `config/config.json-example` to match (drop `api_key`/`pool_address`, add the new fields):

```json
{
    "rpc_url": "https://rpc-testnet.nimiqcloud.com",
    "network": "test-albatross",
    "pool_fee_wallet": "NQ73 ABCD EFGH IJKL MNOP QRST UVWX YZ01 2345",
    "pool_fee_percentage": 0.01,
    "private_key": "your_private_key_here",
    "payout_mode": "delegate",
    "min_payout_luna": 100000,
    "auto_reactivate": true
}
```

- [ ] **Step 3: Write the chain client wiring**

Create `internal/chain/client.go`:

```go
// Package chain builds the nimiq-go RPC client and signer this pool uses to
// talk to a node — the one place that turns config into a live connection.
package chain

import (
	"fmt"

	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/NimMiniApps/nimiq-go/rpc"
	"github.com/NimMiniApps/nimiq-go/signer"

	"github.com/Beardsoft/GoPool/internal/config"
)

type Chain struct {
	RPC     *rpc.Client
	Signer  *signer.PrivateKey
	Network nimiq.NetworkID
}

func networkFromString(s string) (nimiq.NetworkID, error) {
	switch s {
	case "main-albatross":
		return nimiq.NetworkMainAlbatross, nil
	case "test-albatross":
		return nimiq.NetworkTestAlbatross, nil
	case "dev-albatross":
		return nimiq.NetworkDevAlbatross, nil
	case "unit-albatross":
		return nimiq.NetworkUnitAlbatross, nil
	}
	return 0, fmt.Errorf("chain: unknown network %q", s)
}

func New(cfg *config.Config) (*Chain, error) {
	network, err := networkFromString(cfg.Network)
	if err != nil {
		return nil, err
	}
	key, err := signer.ParsePrivateKeyHex(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("chain: parsing private key: %w", err)
	}
	client, err := rpc.New(cfg.RPCURL, rpc.WithNetwork(network))
	if err != nil {
		return nil, fmt.Errorf("chain: %w", err)
	}
	return &Chain{RPC: client, Signer: key, Network: network}, nil
}

// Address is the pool's own validator/fee-paying address, derived from the
// configured private key — no importRawKey/unlockAccount RPC dance needed,
// the SDK signs offline.
func (c *Chain) Address() nimiq.Address {
	return c.Signer.Address()
}
```

- [ ] **Step 4: Delete the now-unused packages**

Run: `grep -rl 'internal/helper' --include=*.go .` — if the only hits are inside `internal/rpc`, delete `internal/helper/` too.

```bash
rm -rf internal/rpc internal/models
```

(`internal/helper` deleted as well if the grep above confirms it.)

- [ ] **Step 5: Verify the config and chain packages build**

Run: `go build ./internal/config/... ./internal/chain/...`
Expected: succeeds. (`go build ./...` will still fail — `internal/pool` and `cmd` still reference the deleted packages until Tasks 3–9.)

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/config internal/chain config/config.json-example
git rm -r internal/rpc internal/models
git commit -m "feat(config): rewrite config and add nimiq-go chain client, drop hand-rolled RPC"
```

---

## Task 3: Main loop and height classification

**Files:**
- Rewrite: `internal/pool/pool.go`
- Test: `internal/pool/pool_test.go`

**Interfaces:**
- Consumes: `chain.Chain` (Task 2), `db.Queries` (Task 1), `rpc.Client.GetPolicy`/`BlockNumber` (nimiq-go).
- Produces:
  - `type Manager struct { chain *chain.Chain; queries *db.Queries; cfg *config.Config; policy *rpc.Policy }`
  - `func NewManager(c *chain.Chain, q *db.Queries, cfg *config.Config) *Manager`
  - `func (m *Manager) Run(ctx context.Context) error` — the ticker loop.
  - `func (m *Manager) classify(height uint32) blockKind` where `blockKind` is `blockMicro | blockCheckpoint | blockElection`, exported as `BlockMicro`/`BlockCheckpoint`/`BlockElection` — later tasks branch on this.
  - `func (m *Manager) processHeight(ctx context.Context, height uint32) error` — dispatches to `handleElection`/`handleCheckpoint` (Tasks 4–5 add the bodies; this task adds the dispatch with no-op stubs it will call, replaced by real implementations in place — not left as a TODO, since Task 3 alone is not meant to run standalone).

Because `processHeight` calls functions Tasks 4–5 define, this task's own test only covers `classify`, which needs no dependency on those.

- [ ] **Step 1: Write the failing test**

Create `internal/pool/pool_test.go`:

```go
package pool

import (
	"testing"

	"github.com/NimMiniApps/nimiq-go/rpc"
)

func testPolicy() *rpc.Policy {
	return &rpc.Policy{
		BlocksPerBatch:     60,
		BatchesPerEpoch:    720,
		BlocksPerEpoch:     43200,
		GenesisBlockNumber: 0,
	}
}

func TestClassify(t *testing.T) {
	m := &Manager{policy: testPolicy()}
	cases := []struct {
		height uint32
		want   blockKind
	}{
		{60, blockCheckpoint},
		{43200, blockElection},
		{999, blockMicro},
		{120, blockCheckpoint},
	}
	for _, c := range cases {
		if got := m.classify(c.height); got != c.want {
			t.Errorf("classify(%d) = %v, want %v", c.height, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/pool/... -run TestClassify -v`
Expected: FAIL — `Manager`/`classify`/`blockKind` undefined

- [ ] **Step 3: Write the implementation**

Replace `internal/pool/pool.go`:

```go
package pool

import (
	"context"
	"time"

	"github.com/NimMiniApps/nimiq-go/rpc"

	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"

	"go.uber.org/zap"
)

type blockKind int

const (
	blockMicro blockKind = iota
	blockCheckpoint
	blockElection
)

const cursorName = "last_processed_height"

type Manager struct {
	chain   *chain.Chain
	queries *db.Queries
	cfg     *config.Config
	policy  *rpc.Policy
}

func NewManager(c *chain.Chain, q *db.Queries, cfg *config.Config) *Manager {
	return &Manager{chain: c, queries: q, cfg: cfg}
}

// classify reports what kind of block a height is, relative to the cached
// policy constants. A height that is both an epoch and a batch boundary is
// an election block, which takes priority over checkpoint.
func (m *Manager) classify(height uint32) blockKind {
	offset := height - m.policy.GenesisBlockNumber
	if offset%m.policy.BlocksPerEpoch == 0 {
		return blockElection
	}
	if offset%m.policy.BlocksPerBatch == 0 {
		return blockCheckpoint
	}
	return blockMicro
}

// Run is the daemon's main loop: replay every height between the last
// processed cursor and the current chain head, in order, then sleep.
func (m *Manager) Run(ctx context.Context) error {
	policy, err := m.chain.RPC.GetPolicy(ctx)
	if err != nil {
		return err
	}
	m.policy = policy

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		head, err := m.chain.RPC.BlockNumber(ctx)
		if err != nil {
			logger.Logger.Error("fetching block number", zap.Error(err))
			continue
		}

		cursor, err := m.queries.GetCursor(ctx, cursorName)
		if err != nil {
			cursor = int64(m.policy.GenesisBlockNumber)
		}

		for h := uint32(cursor) + 1; h <= head; h++ {
			if err := m.processHeight(ctx, h); err != nil {
				logger.Logger.Error("processing height", zap.Uint32("height", h), zap.Error(err))
				break
			}
			if err := m.queries.UpsertCursor(ctx, db.UpsertCursorParams{Name: cursorName, Height: int64(h)}); err != nil {
				logger.Logger.Error("advancing cursor", zap.Error(err))
				break
			}
		}
	}
}

// processHeight dispatches a single height to election/checkpoint handling.
// Micro blocks with no election/checkpoint significance are a no-op — the
// cursor still advances past them so the loop never reprocesses a height.
func (m *Manager) processHeight(ctx context.Context, height uint32) error {
	switch m.classify(height) {
	case blockElection:
		return m.handleElection(ctx, height)
	case blockCheckpoint:
		return m.handleCheckpoint(ctx, height)
	}
	return nil
}
```

`handleElection` and `handleCheckpoint` are defined in Tasks 4 and 5 — this file will not compile stand-alone until those land. That is expected; `go build ./...` is only required to pass at the end of Task 5, not after Task 3 alone (this is called out so a reviewer doesn't reject Task 3 for a repo-wide build failure that Task 4 resolves).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/pool/... -run TestClassify -v`
Expected: PASS (the test file only imports `rpc` and this package, and does not call `processHeight`, so it compiles and passes even before Tasks 4–5 exist).

- [ ] **Step 5: Commit**

```bash
git add internal/pool/pool.go internal/pool/pool_test.go
git commit -m "feat(pool): main loop and block-height classification"
```

---

## Task 4: Election handling

**Files:**
- Create: `internal/pool/election.go`
- Test: `internal/pool/election_test.go`

**Interfaces:**
- Consumes: `Manager` (Task 3), `rpc.Client.GetValidator`/`GetStakersByValidatorAddress` (nimiq-go, the latter from the SDK plan), `db.Queries.EpochExists`/`InsertEpoch`/`InsertStaker` (Task 1).
- Produces: `func (m *Manager) handleElection(ctx context.Context, height uint32) error`, and an exported pure helper `func stakerPercentage(stake, total nimiq.Luna) float64` that Task 5's tests can reuse for sanity-checking payout math against the stored snapshot.

- [ ] **Step 1: Write the failing test**

Create `internal/pool/election_test.go`:

```go
package pool

import (
	"testing"

	nimiq "github.com/NimMiniApps/nimiq-go"
)

func TestStakerPercentage(t *testing.T) {
	got := stakerPercentage(nimiq.Luna(25_000_00000), nimiq.Luna(100_000_00000))
	if got < 24.99 || got > 25.01 {
		t.Errorf("percentage = %v, want ~25", got)
	}
}

func TestStakerPercentageZeroTotal(t *testing.T) {
	if got := stakerPercentage(0, 0); got != 0 {
		t.Errorf("percentage of a zero-balance validator = %v, want 0", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/pool/... -run TestStakerPercentage -v`
Expected: FAIL — `stakerPercentage` undefined

- [ ] **Step 3: Write the implementation**

Create `internal/pool/election.go`:

```go
package pool

import (
	"context"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"

	"go.uber.org/zap"
)

// stakerPercentage is a display/estimation ratio only — actual payout
// amounts are computed from integer stake ratios directly in payout.go, not
// from this float, so its precision does not affect any money path.
func stakerPercentage(stake, total nimiq.Luna) float64 {
	if total == 0 {
		return 0
	}
	return float64(stake) / float64(total) * 100
}

// handleElection runs at the last micro block before an election, or the
// election block itself: it snapshots the validator's stakers and their
// share of the total delegated balance for the upcoming epoch. If the
// validator cannot earn rewards next epoch (inactive, retired, jailed, no
// stakers), the epoch is recorded with that status and no snapshot is taken.
func (m *Manager) handleElection(ctx context.Context, height uint32) error {
	nextEpoch := m.policy.EpochAt(height) + 1

	exists, err := m.queries.EpochExists(ctx, int64(nextEpoch))
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	addr := m.chain.Address()
	validator, err := m.chain.RPC.GetValidator(ctx, addr)
	if err != nil {
		return m.queries.InsertEpoch(ctx, db.InsertEpochParams{
			Number: int64(nextEpoch), NumStakers: 0, Balance: 0, Status: "not_elected",
		})
	}

	status := ""
	switch {
	case validator.Retired:
		status = "retired"
	case validator.InactivityFlag != nil && *validator.InactivityFlag > height:
		status = "inactive"
	case validator.JailedFrom != nil && *validator.JailedFrom > height:
		status = "jailed"
	case validator.NumStakers == 0:
		status = "no_stakers"
	}
	if status != "" {
		return m.queries.InsertEpoch(ctx, db.InsertEpochParams{
			Number: int64(nextEpoch), NumStakers: 0, Balance: int64(validator.Balance), Status: status,
		})
	}

	stakers, err := m.chain.RPC.GetStakersByValidatorAddress(ctx, addr)
	if err != nil {
		return err
	}

	if err := m.queries.InsertEpoch(ctx, db.InsertEpochParams{
		Number: int64(nextEpoch), NumStakers: int64(len(stakers)), Balance: int64(validator.Balance), Status: "in_progress",
	}); err != nil {
		return err
	}

	for _, s := range stakers {
		if s.Balance == 0 {
			continue
		}
		if err := m.queries.InsertStaker(ctx, db.InsertStakerParams{
			EpochNumber: int64(nextEpoch),
			Address:     s.Address,
			Stake:       int64(s.Balance),
			Percentage:  stakerPercentage(s.Balance, validator.Balance),
		}); err != nil {
			return err
		}
	}

	logger.Logger.Info("validator elected for next epoch",
		zap.Uint32("epoch", nextEpoch), zap.Int("stakers", len(stakers)))
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/pool/... -run TestStakerPercentage -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/pool/election.go internal/pool/election_test.go
git commit -m "feat(pool): election-block staker snapshot"
```

---

## Task 5: Checkpoint handling (reward collection)

**Files:**
- Create: `internal/pool/checkpoint.go`
- Test: `internal/pool/checkpoint_test.go`

**Interfaces:**
- Consumes: `Manager`, `rpc.Client.GetInherentsByBlockNumber` (SDK plan), `db.Queries.RewardExists`/`InsertReward`/`GetStakersForEpoch`/`InsertPayslip`.
- Produces: `func (m *Manager) handleCheckpoint(ctx context.Context, height uint32) error`, exported pure helper `func splitReward(reward nimiq.Luna, feePercentage float64) (afterFee, fee nimiq.Luna)`.

`splitReward` uses integer arithmetic (`reward * feeBasisPoints / 10000`), not `float64 * float64`, on the actual money path — this is the concrete fix for the precision bug the design spec called out.

- [ ] **Step 1: Write the failing test**

Create `internal/pool/checkpoint_test.go`:

```go
package pool

import (
	"testing"

	nimiq "github.com/NimMiniApps/nimiq-go"
)

func TestSplitReward(t *testing.T) {
	after, fee := splitReward(nimiq.Luna(100_000_00000), 0.01) // 1% fee
	if fee != 1_000_00000 {
		t.Errorf("fee = %d, want %d", fee, 1_000_00000)
	}
	if after != 99_000_00000 {
		t.Errorf("after = %d, want %d", after, 99_000_00000)
	}
	if after+fee != nimiq.Luna(100_000_00000) {
		t.Error("after + fee must equal the original reward exactly")
	}
}

func TestSplitRewardZeroFee(t *testing.T) {
	after, fee := splitReward(nimiq.Luna(500), 0)
	if fee != 0 || after != 500 {
		t.Errorf("after=%d fee=%d, want after=500 fee=0", after, fee)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/pool/... -run TestSplitReward -v`
Expected: FAIL — `splitReward` undefined

- [ ] **Step 3: Write the implementation**

Create `internal/pool/checkpoint.go`:

```go
package pool

import (
	"context"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"

	"go.uber.org/zap"
)

// splitReward divides a reward into the staker share and the pool fee, in
// integer Luna. feePercentage is converted to basis points (x10000) so the
// division happens on integers, never on floats — the fix for the precision
// bug in the amount predating this rewrite.
func splitReward(reward nimiq.Luna, feePercentage float64) (afterFee, fee nimiq.Luna) {
	basisPoints := uint64(feePercentage * 10000)
	fee = nimiq.Luna(uint64(reward) * basisPoints / 10000)
	return reward - fee, fee
}

// handleCheckpoint runs at every checkpoint (batch boundary): it fetches the
// reward inherent for the batch that just closed one batch size back (reward
// inherents for batch N land in batch N+1, per Nimiq's Albatross reward
// timing), splits it, and fans out a payslip per staker using the epoch's
// stored percentages.
func (m *Manager) handleCheckpoint(ctx context.Context, height uint32) error {
	rewardBatch := m.policy.BatchAt(height) - 1
	rewardHeight := height - m.policy.BlocksPerBatch

	exists, err := m.queries.RewardExists(ctx, int64(rewardBatch))
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	inherents, err := m.chain.RPC.GetInherentsByBlockNumber(ctx, rewardHeight)
	if err != nil {
		return err
	}

	addr := m.chain.Address().String()
	var reward nimiq.Luna
	for _, in := range inherents {
		if in.IsReward() && in.ValidatorAddress == addr {
			reward += in.Value
		}
	}

	epoch := m.policy.EpochAt(rewardHeight)
	stakers, err := m.queries.GetStakersForEpoch(ctx, int64(epoch))
	if err != nil {
		return err
	}

	afterFee, fee := splitReward(reward, m.cfg.PoolFeePercentage)
	if err := m.queries.InsertReward(ctx, db.InsertRewardParams{
		BatchNumber: int64(rewardBatch), EpochNumber: int64(epoch),
		Amount: int64(afterFee), PoolFee: int64(fee), NumStakers: int64(len(stakers)),
	}); err != nil {
		return err
	}

	if reward == 0 || len(stakers) == 0 {
		return nil
	}

	var totalStake int64
	for _, s := range stakers {
		totalStake += s.Stake
	}
	if totalStake == 0 {
		return nil
	}

	for _, s := range stakers {
		// Integer stake ratio, not the stored percentage float: this is the
		// actual payout amount, so it must not go through a float division.
		share := nimiq.Luna(uint64(afterFee) * uint64(s.Stake) / uint64(totalStake))
		if share == 0 {
			continue
		}
		if err := m.queries.InsertPayslip(ctx, db.InsertPayslipParams{
			BatchNumber: int64(rewardBatch), Address: s.Address, Amount: int64(share), Status: "pending",
		}); err != nil {
			return err
		}
	}

	logger.Logger.Info("reward collected",
		zap.Int64("batch", int64(rewardBatch)), zap.Uint64("reward", uint64(reward)), zap.Uint64("fee", uint64(fee)))
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/pool/... -run TestSplitReward -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/pool/checkpoint.go internal/pool/checkpoint_test.go
git commit -m "feat(pool): checkpoint reward collection and payslip fan-out"
```

---

## Task 6: Payout batching

**Files:**
- Create: `internal/pool/payout.go`
- Test: `internal/pool/payout_test.go`

**Interfaces:**
- Consumes: `Manager`, `db.Queries.GetEligibleForPayout`/`MarkPayslipsOutForPayment`/`SetPayslipsTransaction`, `rpc.Client.GetStaker`/`SendTransaction`, `nimiq.NewAddStakeTransaction`/`NewBasicTransaction`/`SignTransaction`/`SignStakingTransaction`.
- Produces: `func (m *Manager) runPayouts(ctx context.Context) error`, called once per tick from `Run` (Task 3's loop gets a one-line addition in this task, shown below), plus exported pure helper `func choosePayoutTx(mode string, stillDelegated bool) payoutKind` returning an enum `payoutDelegate | payoutTransfer`.

- [ ] **Step 1: Write the failing test**

Create `internal/pool/payout_test.go`:

```go
package pool

import "testing"

func TestChoosePayoutTx(t *testing.T) {
	cases := []struct {
		mode           string
		stillDelegated bool
		want           payoutKind
	}{
		{"delegate", true, payoutDelegate},
		{"delegate", false, payoutTransfer}, // fixes zpool's TODO: fall back when undelegated
		{"transfer", true, payoutTransfer},
		{"transfer", false, payoutTransfer},
	}
	for _, c := range cases {
		if got := choosePayoutTx(c.mode, c.stillDelegated); got != c.want {
			t.Errorf("choosePayoutTx(%q, %v) = %v, want %v", c.mode, c.stillDelegated, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/pool/... -run TestChoosePayoutTx -v`
Expected: FAIL — `payoutKind`/`choosePayoutTx` undefined

- [ ] **Step 3: Write the implementation**

Create `internal/pool/payout.go`:

```go
package pool

import (
	"context"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"

	"go.uber.org/zap"
)

type payoutKind int

const (
	payoutTransfer payoutKind = iota
	payoutDelegate
)

// choosePayoutTx picks the transaction shape for a payout. "delegate" mode
// restakes (compounding) only while the staker is still delegated to this
// validator; if they undelegated, it falls back to a plain transfer instead
// of restaking into a stale relationship — the fix for zpool's known TODO,
// which always restakes without checking.
func choosePayoutTx(mode string, stillDelegated bool) payoutKind {
	if mode == "delegate" && stillDelegated {
		return payoutDelegate
	}
	return payoutTransfer
}

// runPayouts sums pending payslips per staker, and for every staker whose
// total has crossed the configured minimum, builds and submits one payout
// transaction covering the whole total.
func (m *Manager) runPayouts(ctx context.Context) error {
	eligible, err := m.queries.GetEligibleForPayout(ctx, int64(m.cfg.MinPayoutLuna))
	if err != nil {
		return err
	}

	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return err
	}

	for _, row := range eligible {
		addr, err := nimiq.ParseAddress(row.Address)
		if err != nil {
			logger.Logger.Error("unparseable staker address, skipping", zap.String("address", row.Address), zap.Error(err))
			continue
		}
		amount := nimiq.Luna(row.Total)

		if err := m.queries.MarkPayslipsOutForPayment(ctx, row.Address); err != nil {
			return err
		}

		kind := payoutTransfer
		if m.cfg.PayoutMode == "delegate" {
			staker, err := m.chain.RPC.GetStaker(ctx, addr)
			stillDelegated := err == nil && staker.Delegation == m.chain.Address().String()
			kind = choosePayoutTx(m.cfg.PayoutMode, stillDelegated)
		}

		hash, err := m.submitPayout(ctx, addr, amount, kind, head)
		if err != nil {
			logger.Logger.Error("payout submission failed, will retry next tick", zap.String("address", row.Address), zap.Error(err))
			continue
		}

		if err := m.queries.SetPayslipsTransaction(ctx, db.SetPayslipsTransactionParams{TxHash: hash, Address: row.Address}); err != nil {
			return err
		}
		if err := m.queries.InsertTransaction(ctx, db.InsertTransactionParams{
			Hash: hash, Address: row.Address, Amount: int64(amount), Status: "awaiting_confirmation",
		}); err != nil {
			return err
		}
		logger.Logger.Info("payout submitted", zap.String("staker", row.Address), zap.Uint64("amount", uint64(amount)), zap.String("tx", hash))
	}
	return nil
}

func (m *Manager) submitPayout(ctx context.Context, recipient nimiq.Address, amount nimiq.Luna, kind payoutKind, head uint32) (string, error) {
	sender := m.chain.Address()
	if kind == payoutDelegate {
		tx, err := nimiq.NewAddStakeTransaction(sender, recipient, amount, 0, head, m.chain.Network)
		if err != nil {
			return "", err
		}
		if err := nimiq.SignStakingTransaction(ctx, tx, m.chain.Signer, m.chain.Signer); err != nil {
			return "", err
		}
		return m.chain.RPC.SendTransaction(ctx, tx)
	}
	tx, err := nimiq.NewBasicTransaction(sender, recipient, amount, 0, head, m.chain.Network)
	if err != nil {
		return "", err
	}
	if err := nimiq.SignTransaction(ctx, m.chain.Signer, tx); err != nil {
		return "", err
	}
	return m.chain.RPC.SendTransaction(ctx, tx)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/pool/... -run TestChoosePayoutTx -v`
Expected: PASS

- [ ] **Step 5: Wire `runPayouts` into the main loop**

In `internal/pool/pool.go` (Task 3), inside `Run`'s `for` loop, immediately after the `for h := ...` height-replay loop and before it loops back to `select`, add:

```go
		if err := m.runPayouts(ctx); err != nil {
			logger.Logger.Error("running payouts", zap.Error(err))
		}
```

- [ ] **Step 6: Commit**

```bash
git add internal/pool/payout.go internal/pool/payout_test.go internal/pool/pool.go
git commit -m "feat(pool): payout batching with delegate/transfer fallback"
```

---

## Task 7: Confirmation tracking and epoch finalization

**Files:**
- Create: `internal/pool/confirmation.go`
- Test: `internal/pool/confirmation_test.go`

**Interfaces:**
- Consumes: `Manager`, `rpc.Client.CheckTransaction` (nimiq-go), `db.Queries.GetPendingTransactions`/`SetTransactionStatus`/`FinalizePayslips`/`ResetPayslipsToPending`/`FinalizeCompletedEpochs`.
- Produces: `func (m *Manager) runConfirmations(ctx context.Context) error`, exported pure helper `func confirmationOutcome(conf *rpc.Confirmation) confirmOutcome` returning `outcomePending | outcomeSucceeded | outcomeFailed`.

- [ ] **Step 1: Write the failing test**

Create `internal/pool/confirmation_test.go`:

```go
package pool

import (
	"testing"

	"github.com/NimMiniApps/nimiq-go/rpc"
)

func TestConfirmationOutcome(t *testing.T) {
	cases := []struct {
		name string
		conf *rpc.Confirmation
		want confirmOutcome
	}{
		{"not yet final", &rpc.Confirmation{Included: true, Final: false, Succeeded: true}, outcomePending},
		{"final and succeeded", &rpc.Confirmation{Included: true, Final: true, Succeeded: true}, outcomeSucceeded},
		{"final but failed", &rpc.Confirmation{Included: true, Final: true, Succeeded: false}, outcomeFailed},
	}
	for _, c := range cases {
		if got := confirmationOutcome(c.conf); got != c.want {
			t.Errorf("%s: confirmationOutcome = %v, want %v", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/pool/... -run TestConfirmationOutcome -v`
Expected: FAIL — `confirmOutcome`/`confirmationOutcome` undefined

- [ ] **Step 3: Write the implementation**

Create `internal/pool/confirmation.go`:

```go
package pool

import (
	"context"

	"github.com/NimMiniApps/nimiq-go/rpc"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"

	"go.uber.org/zap"
)

type confirmOutcome int

const (
	outcomePending confirmOutcome = iota
	outcomeSucceeded
	outcomeFailed
)

// confirmationOutcome classifies a Confirmation. Only a macro-final result is
// decisive — a micro-block-included transaction under Albatross can still be
// reverted, so "pending" covers included-but-not-final as well as
// not-yet-included.
func confirmationOutcome(conf *rpc.Confirmation) confirmOutcome {
	if !conf.Final {
		return outcomePending
	}
	if conf.Succeeded {
		return outcomeSucceeded
	}
	return outcomeFailed
}

// runConfirmations settles every pending payout transaction: succeeded ones
// finalize their payslips, failed ones reset their payslips to pending for
// retry next payout pass. It then rolls any epoch whose every payslip is
// settled into "completed".
func (m *Manager) runConfirmations(ctx context.Context) error {
	pending, err := m.queries.GetPendingTransactions(ctx)
	if err != nil {
		return err
	}

	for _, tx := range pending {
		conf, err := m.chain.RPC.CheckTransaction(ctx, tx.Hash)
		if err != nil {
			logger.Logger.Error("checking transaction", zap.String("hash", tx.Hash), zap.Error(err))
			continue
		}
		switch confirmationOutcome(conf) {
		case outcomePending:
			continue
		case outcomeSucceeded:
			if err := m.queries.SetTransactionStatus(ctx, db.SetTransactionStatusParams{Status: "completed", Hash: tx.Hash}); err != nil {
				return err
			}
			if err := m.queries.FinalizePayslips(ctx, tx.Hash); err != nil {
				return err
			}
			logger.Logger.Info("payout confirmed", zap.String("hash", tx.Hash))
		case outcomeFailed:
			if err := m.queries.SetTransactionStatus(ctx, db.SetTransactionStatusParams{Status: "failed", Hash: tx.Hash}); err != nil {
				return err
			}
			if err := m.queries.ResetPayslipsToPending(ctx, tx.Hash); err != nil {
				return err
			}
			logger.Logger.Warn("payout failed, reset for retry", zap.String("hash", tx.Hash))
		}
	}

	completed, err := m.queries.FinalizeCompletedEpochs(ctx)
	if err != nil {
		return err
	}
	for _, number := range completed {
		logger.Logger.Info("epoch finalized", zap.Int64("epoch", number))
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/pool/... -run TestConfirmationOutcome -v`
Expected: PASS

- [ ] **Step 5: Wire into the main loop**

In `internal/pool/pool.go`'s `Run`, alongside the `runPayouts` call added in Task 6:

```go
		if err := m.runConfirmations(ctx); err != nil {
			logger.Logger.Error("running confirmations", zap.Error(err))
		}
```

- [ ] **Step 6: Commit**

```bash
git add internal/pool/confirmation.go internal/pool/confirmation_test.go internal/pool/pool.go
git commit -m "feat(pool): transaction confirmation tracking and epoch finalization"
```

---

## Task 8: Validator lifecycle — auto-reactivate and CLI actions

**Files:**
- Create: `internal/pool/lifecycle.go`
- Test: `internal/pool/lifecycle_test.go`

**Interfaces:**
- Consumes: `Manager`, `rpc.Client.GetValidator`, `nimiq.NewReactivateValidatorTransaction`/`NewDeactivateValidatorTransaction`/`NewRetireValidatorTransaction`/`NewDeleteValidatorTransaction` (SDK plan), `db.Queries.HasPendingValidatorAction`/`InsertValidatorAction`.
- Produces:
  - `func (m *Manager) runAutoReactivate(ctx context.Context) error` — called from `Run` only if `cfg.AutoReactivate`.
  - `func (m *Manager) Deactivate(ctx context.Context) error`, `func (m *Manager) Retire(ctx context.Context) error`, `func (m *Manager) Delete(ctx context.Context, recipient nimiq.Address) error` — called directly by the CLI in Task 9, not from the daemon loop.
  - exported pure helper `func shouldReactivate(jailedFrom *uint32, height uint32) bool`.

- [ ] **Step 1: Write the failing test**

Create `internal/pool/lifecycle_test.go`:

```go
package pool

import "testing"

func TestShouldReactivate(t *testing.T) {
	jailedUntil := uint32(1000)
	cases := []struct {
		name        string
		jailedFrom  *uint32
		height      uint32
		want        bool
	}{
		{"not jailed", nil, 2000, false},
		{"still jailed", &jailedUntil, 500, false},
		{"cooldown passed", &jailedUntil, 1000, true},
		{"well past cooldown", &jailedUntil, 5000, true},
	}
	for _, c := range cases {
		if got := shouldReactivate(c.jailedFrom, c.height); got != c.want {
			t.Errorf("%s: shouldReactivate = %v, want %v", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/pool/... -run TestShouldReactivate -v`
Expected: FAIL — `shouldReactivate` undefined

- [ ] **Step 3: Write the implementation**

Create `internal/pool/lifecycle.go`:

```go
package pool

import (
	"context"
	"fmt"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"

	"go.uber.org/zap"
)

// shouldReactivate reports whether a jailed validator's cooldown has elapsed.
// A nil jailedFrom means the validator is not jailed at all.
func shouldReactivate(jailedFrom *uint32, height uint32) bool {
	if jailedFrom == nil {
		return false
	}
	return height >= *jailedFrom
}

const actionReactivate = "reactivate"

// runAutoReactivate sends a ReactivateValidator transaction once, the first
// tick after a jail cooldown elapses. Guarded against resending every tick
// by checking for an unconfirmed action row first — see the "pending"
// outcome recorded in Step 3 below and cleared once the daemon observes the
// validator is no longer jailed.
func (m *Manager) runAutoReactivate(ctx context.Context) error {
	addr := m.chain.Address()
	validator, err := m.chain.RPC.GetValidator(ctx, addr)
	if err != nil {
		return nil // not a validator (yet); nothing to reactivate
	}

	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return err
	}
	if !shouldReactivate(validator.JailedFrom, head) {
		return nil
	}

	pending, err := m.queries.HasPendingValidatorAction(ctx, actionReactivate)
	if err != nil {
		return err
	}
	if pending {
		return nil
	}

	tx, err := nimiq.NewReactivateValidatorTransaction(addr, addr, 0, head, m.chain.Network)
	if err != nil {
		return err
	}
	if err := nimiq.SignStakingTransaction(ctx, tx, m.chain.Signer, m.chain.Signer); err != nil {
		return err
	}
	hash, err := m.chain.RPC.SendTransaction(ctx, tx)
	outcome := "pending"
	if err != nil {
		outcome = "failed"
	}
	if insertErr := m.queries.InsertValidatorAction(ctx, db.InsertValidatorActionParams{
		Action: actionReactivate, TxHash: hash, Outcome: outcome,
	}); insertErr != nil {
		return insertErr
	}
	if err != nil {
		return err
	}
	logger.Logger.Info("validator reactivation submitted", zap.String("tx", hash))
	return nil
}

// Deactivate, Retire, and Delete are operator-triggered, run from the CLI in
// cmd/main.go — never from the daemon loop. Deactivating, retiring, or
// deleting a validator affects delegator funds and pool operation; a human
// must choose to do it.

func (m *Manager) Deactivate(ctx context.Context) error {
	addr := m.chain.Address()
	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return err
	}
	tx, err := nimiq.NewDeactivateValidatorTransaction(addr, addr, 0, head, m.chain.Network)
	if err != nil {
		return err
	}
	if err := nimiq.SignStakingTransaction(ctx, tx, m.chain.Signer, m.chain.Signer); err != nil {
		return err
	}
	hash, err := m.chain.RPC.SendTransaction(ctx, tx)
	if err != nil {
		return err
	}
	fmt.Printf("deactivate transaction submitted: %s\n", hash)
	return nil
}

func (m *Manager) Retire(ctx context.Context) error {
	addr := m.chain.Address()
	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return err
	}
	tx, err := nimiq.NewRetireValidatorTransaction(addr, 0, head, m.chain.Network)
	if err != nil {
		return err
	}
	if err := nimiq.SignStakingTransaction(ctx, tx, m.chain.Signer, m.chain.Signer); err != nil {
		return err
	}
	hash, err := m.chain.RPC.SendTransaction(ctx, tx)
	if err != nil {
		return err
	}
	fmt.Printf("retire transaction submitted: %s\n", hash)
	return nil
}

func (m *Manager) Delete(ctx context.Context, recipient nimiq.Address, value nimiq.Luna) error {
	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return err
	}
	tx, err := nimiq.NewDeleteValidatorTransaction(recipient, value, 0, head, m.chain.Network)
	if err != nil {
		return err
	}
	if err := nimiq.SignTransaction(ctx, m.chain.Signer, tx); err != nil {
		return err
	}
	hash, err := m.chain.RPC.SendTransaction(ctx, tx)
	if err != nil {
		return err
	}
	fmt.Printf("delete transaction submitted: %s\n", hash)
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/pool/... -run TestShouldReactivate -v`
Expected: PASS

- [ ] **Step 5: Wire auto-reactivate into the main loop**

In `internal/pool/pool.go`'s `Run`, alongside the other per-tick calls added in Tasks 6–7, gated on config:

```go
		if m.cfg.AutoReactivate {
			if err := m.runAutoReactivate(ctx); err != nil {
				logger.Logger.Error("running auto-reactivate", zap.Error(err))
			}
		}
```

- [ ] **Step 6: Commit**

```bash
git add internal/pool/lifecycle.go internal/pool/lifecycle_test.go internal/pool/pool.go
git commit -m "feat(pool): validator lifecycle - auto-reactivate and operator-triggered actions"
```

---

## Task 9: CLI entrypoint and daemon wiring

**Files:**
- Rewrite: `cmd/main.go`

**Interfaces:**
- Consumes: everything from Tasks 1–8: `config.LoadConfig`, `chain.New`, `db.InitDB`/`db.New`, `pool.NewManager`, `(*pool.Manager).Run`/`Deactivate`/`Retire`/`Delete`.
- Produces: the `gopool` binary's two entrypoints — daemon (no args) and `gopool validator deactivate|retire|delete <recipient> <value>`.

- [ ] **Step 1: Rewrite `cmd/main.go`**

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"
	"github.com/Beardsoft/GoPool/internal/pool"

	"go.uber.org/zap"
)

func main() {
	logger.InitLogger()
	defer logger.Sync()

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Logger.Fatal("failed to load config", zap.Error(err))
	}

	c, err := chain.New(cfg)
	if err != nil {
		logger.Logger.Fatal("failed to set up chain client", zap.Error(err))
	}

	sqlDB, err := db.InitDB("pool.db")
	if err != nil {
		logger.Logger.Fatal("failed to initialize the database", zap.Error(err))
	}
	defer sqlDB.Close()
	queries := db.New(sqlDB)

	manager := pool.NewManager(c, queries, cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if len(os.Args) > 1 && os.Args[1] == "validator" {
		if err := runValidatorCLI(ctx, manager, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	if err := manager.Run(ctx); err != nil {
		logger.Logger.Error("pool manager stopped", zap.Error(err))
	}
}

func runValidatorCLI(ctx context.Context, m *pool.Manager, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: gopool validator deactivate|retire|delete <recipient> <value-luna>")
	}
	switch args[0] {
	case "deactivate":
		return m.Deactivate(ctx)
	case "retire":
		return m.Retire(ctx)
	case "delete":
		if len(args) != 3 {
			return fmt.Errorf("usage: gopool validator delete <recipient> <value-luna>")
		}
		recipient, err := nimiq.ParseAddress(args[1])
		if err != nil {
			return err
		}
		var value uint64
		if _, err := fmt.Sscanf(args[2], "%d", &value); err != nil {
			return fmt.Errorf("invalid value %q: %w", args[2], err)
		}
		return m.Delete(ctx, recipient, nimiq.Luna(value))
	}
	return fmt.Errorf("unknown validator action %q", args[0])
}
```

- [ ] **Step 2: Build the whole module**

Run: `go build ./...`
Expected: succeeds — this is the first point every deleted-package reference, every generated-query call site, and every task's wiring must actually compile together.

- [ ] **Step 3: Run the full test suite**

Run: `go test ./... -v`
Expected: PASS for every test added in Tasks 3–8.

- [ ] **Step 4: Commit**

```bash
git add cmd/main.go
git commit -m "feat(cmd): daemon entrypoint and validator CLI subcommand"
```

---

## Task 10: Devnet end-to-end smoke test

**Files:** none (manual verification, no code changes)

**Interfaces:** none — this task validates Tasks 1–9 together against a live devnet, the way `README.md`'s existing "Run the pool" instructions describe doing against a real node.

**Never run any step of this task against mainnet.**

- [ ] **Step 1: Stand up a devnet**

Use GoPool's own `devlab/` setup or zpool's `devnet.config.toml` as a starting point for a disposable test-albatross network with one validator whose signing/private keys you control.

- [ ] **Step 2: Configure and run the pool against it**

Copy `config/config.json-example` to `config.json`, fill in the devnet RPC URL, `network: "dev-albatross"` or `"test-albatross"` to match the devnet, and the devnet validator's private key. Run:

```bash
go run ./cmd
```

- [ ] **Step 3: Verify the daemon reaches steady state**

Watch the logs for at least one full epoch: confirm "validator elected for next epoch" fires at election, "reward collected" fires at each checkpoint if the devnet validator earns anything, and (once a staker is delegated on the devnet and crosses `min_payout_luna`) "payout submitted" followed later by "payout confirmed".

- [ ] **Step 4: Verify the CLI subcommands against the same devnet**

```bash
go run ./cmd validator deactivate
```

Confirm via `getValidatorByAddress` on the devnet RPC that `inactivityFlag` is now set, then run `go run ./cmd validator retire` after deactivation settles, and confirm `retired: true`. Do not run `validator delete` unless the plan's goal is specifically to test deposit reclaim, since it removes the validator from the devnet permanently.

- [ ] **Step 5: Record findings**

If any step in Tasks 1–9 needed a correction to match real devnet behavior (a query, a payload shape, a status transition), open a short note in `docs/superpowers/plans/2026-08-12-validator-pool-daemon.md` (this file) under a new `## Devnet verification notes` heading at the end, rather than silently patching code without a record of why.
