# GoPool API + Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a read/operator API (`cmd/api`) and a Vue frontend (`web/`) on top of the existing pool daemon, so stakers can look up their stake/payout history and the operator can check daemon health and trigger deactivate/retire without SSH+CLI.

**Architecture:** `cmd/api` is a new, separate Go binary from the daemon (`cmd`), sharing the daemon's SQLite file. It never holds the validator's private key. Data endpoints read `internal/db` tables directly. Operator actions (deactivate/retire) are never signed by the API — it inserts a `validator_actions` row with `outcome = 'requested'`, and the daemon's existing main loop (`internal/pool/pool.go`) picks it up, signs, and submits it, exactly like every other daemon-initiated transaction. Auth for both stakers and the operator is Nimiq Hub signed-message login (`nimiq.VerifyMessageFrom`), so there is no separate password/secret to provision — "operator" is defined as "controls the validator address," the same identity the daemon itself already uses. The frontend is a Vue 3 SPA using the Nimiq UI Kit token set, built with Vite and embedded into the `cmd/api` binary via `go:embed`, so there is one artifact to deploy alongside the daemon.

**Tech Stack:** Go 1.25 stdlib (`net/http` with the Go 1.22+ pattern-based `ServeMux` — no router dependency), existing `internal/db` (sqlc-style, sqlite), `github.com/NimMiniApps/nimiq-go` (already a dependency) for Hub-login verification. Frontend: Vue 3, Vite, `vue-router`, `@nimiq/hub-api` for the client-side Hub login popup, Nimiq UI Kit's `tokens.css` for styling (no `@nimiq/vue-components` — this is a new app with modest UI needs, so hand-rolled components styled with kit tokens are enough; see `nimiq-ui-kit` skill guidance to only pull in the component library when a project already depends on it).

## Global Constraints

- Go version floor: 1.25 (matches `go.mod`; do not introduce a Go dependency requiring a newer toolchain).
- No new Go dependencies. Everything server-side is stdlib + the existing `nimiq-go`, `go-sqlite3`, `zap`, `viper` already in `go.mod`.
- `sqlc` is not installed in this environment (`which sqlc` → not found). New query functions in `internal/db/queries.sql.go` must be hand-written to exactly match the existing generated style (see the file's current contents for the pattern: `const <Name> = "..."`, an optional `<Name>Row` struct, a `func (q *Queries) <Name>(...)`). If a future environment has `sqlc` installed, running `sqlc generate` from the additions in `sql/queries.sql` must reproduce equivalent code — do not hand-write anything sqlc could not generate.
- The validator's private key is loaded in exactly one process: the daemon (`cmd`). `cmd/api` must never call `signer.ParsePrivateKeyHex` or read `cfg.PrivateKey`.
- Follow existing code conventions: `zap` structured logging via `internal/logger`, table-driven Go tests, doc comments only where they explain a non-obvious "why" (see `internal/pool/*.go` for the house style).
- All new SQLite tables/columns: none needed. This plan reuses `validator_actions.outcome` (already free-form TEXT) with a new value, `"requested"`, rather than adding a column — see Task 2.

---

## File Structure

```
internal/config/config.go        MODIFY: add APIAddr, SessionSecret, ValidatorAddress + ValidateAPI
internal/chain/client.go         MODIFY: add NewRPCOnly (no signer)
internal/pool/pool.go            MODIFY: export CursorName, wire ProcessRequestedActions into Run()
internal/pool/lifecycle.go       MODIFY: Deactivate/Retire return (string, error); add ProcessRequestedActions
internal/pool/lifecycle_test.go  MODIFY: cover the refactored return values and the new poll step
cmd/main.go                      MODIFY: update CLI call sites for the new Deactivate/Retire signature
sql/queries.sql                  MODIFY: append validator-action polling + API read queries
internal/db/queries.sql.go       MODIFY: append hand-written equivalents (sqlc style)

cmd/api/main.go                  CREATE: API entrypoint
internal/api/api.go              CREATE: API struct, New(), Mux()
internal/api/pool_handlers.go    CREATE: GET /api/pool
internal/api/epoch_handlers.go   CREATE: GET /api/epochs, GET /api/epochs/{number}
internal/api/staker_handlers.go  CREATE: GET /api/stakers/{address}, GET /api/me
internal/api/auth.go             CREATE: nonce store, challenge/verify, session cookie, middleware
internal/api/operator_handlers.go CREATE: GET /api/operator/health, POST .../deactivate, .../retire
internal/api/embed.go            CREATE: go:embed of web/dist + SPA file serving
internal/api/testdb.go           CREATE: test-only in-memory sqlite helper (build-tagged test helper file, or _test.go per package — see Task 3)

web/package.json, vite.config.ts, index.html, tsconfig.json   CREATE: Vite + Vue 3 scaffold
web/src/main.ts, App.vue, router.ts, api.ts, hub.ts, style.css CREATE
web/src/pages/PoolOverview.vue, Epochs.vue, EpochDetail.vue    CREATE
web/src/pages/StakerLookup.vue, MyDashboard.vue, Operator.vue  CREATE

config/config.json-example        MODIFY: document the three new fields
```

---

## Task 1: Config, read-only chain client, and DB queries

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/chain/client.go`
- Modify: `internal/pool/pool.go:26` (the `cursorName` const)
- Modify: `sql/queries.sql`
- Modify: `internal/db/queries.sql.go`
- Modify: `config/config.json-example`
- Test: `internal/config/config_test.go` (new)

**Interfaces:**
- Produces: `config.Config.APIAddr`, `.SessionSecret`, `.ValidatorAddress` fields; `config.ValidateAPI(cfg *Config) error`; `chain.NewRPCOnly(cfg *config.Config) (*rpc.Client, error)`; `pool.CursorName` (exported, was `cursorName`); `db.Queries.GetRequestedValidatorActions`, `.SetValidatorActionOutcome`, `.HasOutstandingValidatorAction`, `.ListEpochs`, `.GetEpochByNumber`, `.GetLatestEpoch`, `.SumRewardsAmount`, `.StakerExists`, `.GetStakerLatest`, `.GetPayslipsForAddress`, `.GetTransactionsForAddress`, `.GetStuckPayslips` (all with their `*Row` types as defined below).

- [ ] **Step 1: Write the failing config test**

Create `internal/config/config_test.go`:

```go
package config

import "testing"

func TestValidateAPI(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"all set", Config{APIAddr: ":8080", SessionSecret: "s3cr3t", ValidatorAddress: "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"}, false},
		{"missing api_addr", Config{SessionSecret: "s3cr3t", ValidatorAddress: "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"}, true},
		{"missing session_secret", Config{APIAddr: ":8080", ValidatorAddress: "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"}, true},
		{"missing validator_address", Config{APIAddr: ":8080", SessionSecret: "s3cr3t"}, true},
		{"invalid validator_address", Config{APIAddr: ":8080", SessionSecret: "s3cr3t", ValidatorAddress: "not-an-address"}, true},
	}
	for _, c := range cases {
		err := ValidateAPI(&c.cfg)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: ValidateAPI() error = %v, wantErr %v", c.name, err, c.wantErr)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/config/... -run TestValidateAPI -v`
Expected: FAIL — `ValidateAPI` and the three new fields do not exist yet (compile error).

- [ ] **Step 3: Add the config fields and ValidateAPI**

In `internal/config/config.go`, add to the `Config` struct (after `AutoReactivate`):

```go
	// APIAddr, SessionSecret, and ValidatorAddress are only used by cmd/api.
	// The daemon (cmd) ignores them.
	APIAddr          string `mapstructure:"api_addr"`
	SessionSecret    string `mapstructure:"session_secret"`
	ValidatorAddress string `mapstructure:"validator_address"`
```

Add, after `Validate`:

```go
// ValidateAPI reports whether the API-only config fields are present and
// well-formed. The daemon's LoadConfig/Validate does not require these —
// only cmd/api calls this, right after loading config.
func ValidateAPI(c *Config) error {
	if c.APIAddr == "" {
		return fmt.Errorf("config: api_addr is required")
	}
	if c.SessionSecret == "" {
		return fmt.Errorf("config: session_secret is required")
	}
	if c.ValidatorAddress == "" {
		return fmt.Errorf("config: validator_address is required")
	}
	if _, err := nimiq.ParseAddress(c.ValidatorAddress); err != nil {
		return fmt.Errorf("config: validator_address: %w", err)
	}
	return nil
}
```

Add the import: `nimiq "github.com/NimMiniApps/nimiq-go"`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/config/... -run TestValidateAPI -v`
Expected: PASS

- [ ] **Step 5: Add chain.NewRPCOnly**

In `internal/chain/client.go`, add after `New`:

```go
// NewRPCOnly builds an RPC client with no signer, for callers — like the API
// — that must never hold the validator's private key. It reuses the same
// network parsing New does, so RPC behavior (network ID, endpoint) is
// identical either way.
func NewRPCOnly(cfg *config.Config) (*rpc.Client, error) {
	network, err := networkFromString(cfg.Network)
	if err != nil {
		return nil, err
	}
	client, err := rpc.New(cfg.RPCURL, rpc.WithNetwork(network))
	if err != nil {
		return nil, fmt.Errorf("chain: %w", err)
	}
	return client, nil
}
```

- [ ] **Step 6: Export the daemon's cursor name**

In `internal/pool/pool.go`, rename the constant and its two usages:

```go
const CursorName = "last_processed_height"
```

Update the two references in `Run` (`db.GetCursor(ctx, cursorName)` → `db.GetCursor(ctx, CursorName)`, `db.UpsertCursorParams{Name: cursorName, ...}` → `db.UpsertCursorParams{Name: CursorName, ...}`). This lets `internal/api`'s health endpoint (Task 8) read the same cursor row without duplicating the string literal.

- [ ] **Step 7: Add the new SQL queries**

Append to `sql/queries.sql`:

```sql
-- name: GetRequestedValidatorActions :many
SELECT id, action FROM validator_actions WHERE outcome = 'requested' ORDER BY id;

-- name: SetValidatorActionOutcome :exec
UPDATE validator_actions SET outcome = ?, tx_hash = ? WHERE id = ?;

-- name: HasOutstandingValidatorAction :one
SELECT EXISTS(SELECT 1 FROM validator_actions WHERE action = ? AND outcome IN ('requested', 'pending'));

-- name: ListEpochs :many
SELECT number, num_stakers, balance, status FROM epochs ORDER BY number DESC;

-- name: GetEpochByNumber :one
SELECT number, num_stakers, balance, status FROM epochs WHERE number = ?;

-- name: GetLatestEpoch :one
SELECT number, num_stakers, balance, status FROM epochs ORDER BY number DESC LIMIT 1;

-- name: SumRewardsAmount :one
SELECT CAST(COALESCE(SUM(amount), 0) AS INTEGER) FROM rewards;

-- name: StakerExists :one
SELECT EXISTS(SELECT 1 FROM stakers WHERE address = ?);

-- name: GetStakerLatest :one
SELECT epoch_number, stake, percentage FROM stakers WHERE address = ? ORDER BY epoch_number DESC LIMIT 1;

-- name: GetPayslipsForAddress :many
SELECT batch_number, amount, status, tx_hash FROM payslips WHERE address = ? ORDER BY batch_number DESC;

-- name: GetTransactionsForAddress :many
SELECT hash, amount, status, submitted_at FROM transactions WHERE address = ? ORDER BY submitted_at DESC;

-- name: GetStuckPayslips :many
SELECT p.id, p.batch_number, p.address, p.amount, p.status, p.tx_hash, t.submitted_at
FROM payslips p
LEFT JOIN transactions t ON t.hash = p.tx_hash
WHERE p.status IN ('out_for_payment', 'awaiting_confirmation')
ORDER BY p.id;
```

- [ ] **Step 8: Add the matching hand-written Go**

Append to `internal/db/queries.sql.go` (each block placed alphabetically by query name among the existing ones, matching sqlc's own ordering convention — see the file's current layout):

```go
const GetEpochByNumber = `-- name: GetEpochByNumber :one
SELECT number, num_stakers, balance, status FROM epochs WHERE number = ?
`

type GetEpochByNumberRow struct {
	Number     int64  `json:"number"`
	NumStakers int64  `json:"num_stakers"`
	Balance    int64  `json:"balance"`
	Status     string `json:"status"`
}

func (q *Queries) GetEpochByNumber(ctx context.Context, number int64) (GetEpochByNumberRow, error) {
	row := q.db.QueryRowContext(ctx, GetEpochByNumber, number)
	var i GetEpochByNumberRow
	err := row.Scan(&i.Number, &i.NumStakers, &i.Balance, &i.Status)
	return i, err
}

const GetLatestEpoch = `-- name: GetLatestEpoch :one
SELECT number, num_stakers, balance, status FROM epochs ORDER BY number DESC LIMIT 1
`

type GetLatestEpochRow struct {
	Number     int64  `json:"number"`
	NumStakers int64  `json:"num_stakers"`
	Balance    int64  `json:"balance"`
	Status     string `json:"status"`
}

func (q *Queries) GetLatestEpoch(ctx context.Context) (GetLatestEpochRow, error) {
	row := q.db.QueryRowContext(ctx, GetLatestEpoch)
	var i GetLatestEpochRow
	err := row.Scan(&i.Number, &i.NumStakers, &i.Balance, &i.Status)
	return i, err
}

const GetPayslipsForAddress = `-- name: GetPayslipsForAddress :many
SELECT batch_number, amount, status, tx_hash FROM payslips WHERE address = ? ORDER BY batch_number DESC
`

type GetPayslipsForAddressRow struct {
	BatchNumber int64          `json:"batch_number"`
	Amount      int64          `json:"amount"`
	Status      string         `json:"status"`
	TxHash      sql.NullString `json:"tx_hash"`
}

func (q *Queries) GetPayslipsForAddress(ctx context.Context, address string) ([]GetPayslipsForAddressRow, error) {
	rows, err := q.db.QueryContext(ctx, GetPayslipsForAddress, address)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []GetPayslipsForAddressRow{}
	for rows.Next() {
		var i GetPayslipsForAddressRow
		if err := rows.Scan(&i.BatchNumber, &i.Amount, &i.Status, &i.TxHash); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const GetRequestedValidatorActions = `-- name: GetRequestedValidatorActions :many
SELECT id, action FROM validator_actions WHERE outcome = 'requested' ORDER BY id
`

type GetRequestedValidatorActionsRow struct {
	ID     int64  `json:"id"`
	Action string `json:"action"`
}

func (q *Queries) GetRequestedValidatorActions(ctx context.Context) ([]GetRequestedValidatorActionsRow, error) {
	rows, err := q.db.QueryContext(ctx, GetRequestedValidatorActions)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []GetRequestedValidatorActionsRow{}
	for rows.Next() {
		var i GetRequestedValidatorActionsRow
		if err := rows.Scan(&i.ID, &i.Action); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const GetStakerLatest = `-- name: GetStakerLatest :one
SELECT epoch_number, stake, percentage FROM stakers WHERE address = ? ORDER BY epoch_number DESC LIMIT 1
`

type GetStakerLatestRow struct {
	EpochNumber int64   `json:"epoch_number"`
	Stake       int64   `json:"stake"`
	Percentage  float64 `json:"percentage"`
}

func (q *Queries) GetStakerLatest(ctx context.Context, address string) (GetStakerLatestRow, error) {
	row := q.db.QueryRowContext(ctx, GetStakerLatest, address)
	var i GetStakerLatestRow
	err := row.Scan(&i.EpochNumber, &i.Stake, &i.Percentage)
	return i, err
}

const GetStuckPayslips = `-- name: GetStuckPayslips :many
SELECT p.id, p.batch_number, p.address, p.amount, p.status, p.tx_hash, t.submitted_at
FROM payslips p
LEFT JOIN transactions t ON t.hash = p.tx_hash
WHERE p.status IN ('out_for_payment', 'awaiting_confirmation')
ORDER BY p.id
`

type GetStuckPayslipsRow struct {
	ID          int64          `json:"id"`
	BatchNumber int64          `json:"batch_number"`
	Address     string         `json:"address"`
	Amount      int64          `json:"amount"`
	Status      string         `json:"status"`
	TxHash      sql.NullString `json:"tx_hash"`
	SubmittedAt sql.NullTime   `json:"submitted_at"`
}

func (q *Queries) GetStuckPayslips(ctx context.Context) ([]GetStuckPayslipsRow, error) {
	rows, err := q.db.QueryContext(ctx, GetStuckPayslips)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []GetStuckPayslipsRow{}
	for rows.Next() {
		var i GetStuckPayslipsRow
		if err := rows.Scan(&i.ID, &i.BatchNumber, &i.Address, &i.Amount, &i.Status, &i.TxHash, &i.SubmittedAt); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const GetTransactionsForAddress = `-- name: GetTransactionsForAddress :many
SELECT hash, amount, status, submitted_at FROM transactions WHERE address = ? ORDER BY submitted_at DESC
`

type GetTransactionsForAddressRow struct {
	Hash        string       `json:"hash"`
	Amount      int64        `json:"amount"`
	Status      string       `json:"status"`
	SubmittedAt sql.NullTime `json:"submitted_at"`
}

func (q *Queries) GetTransactionsForAddress(ctx context.Context, address string) ([]GetTransactionsForAddressRow, error) {
	rows, err := q.db.QueryContext(ctx, GetTransactionsForAddress, address)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []GetTransactionsForAddressRow{}
	for rows.Next() {
		var i GetTransactionsForAddressRow
		if err := rows.Scan(&i.Hash, &i.Amount, &i.Status, &i.SubmittedAt); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const HasOutstandingValidatorAction = `-- name: HasOutstandingValidatorAction :one
SELECT EXISTS(SELECT 1 FROM validator_actions WHERE action = ? AND outcome IN ('requested', 'pending'))
`

func (q *Queries) HasOutstandingValidatorAction(ctx context.Context, action string) (int64, error) {
	row := q.db.QueryRowContext(ctx, HasOutstandingValidatorAction, action)
	var column_1 int64
	err := row.Scan(&column_1)
	return column_1, err
}

const ListEpochs = `-- name: ListEpochs :many
SELECT number, num_stakers, balance, status FROM epochs ORDER BY number DESC
`

type ListEpochsRow struct {
	Number     int64  `json:"number"`
	NumStakers int64  `json:"num_stakers"`
	Balance    int64  `json:"balance"`
	Status     string `json:"status"`
}

func (q *Queries) ListEpochs(ctx context.Context) ([]ListEpochsRow, error) {
	rows, err := q.db.QueryContext(ctx, ListEpochs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ListEpochsRow{}
	for rows.Next() {
		var i ListEpochsRow
		if err := rows.Scan(&i.Number, &i.NumStakers, &i.Balance, &i.Status); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const SetValidatorActionOutcome = `-- name: SetValidatorActionOutcome :exec
UPDATE validator_actions SET outcome = ?, tx_hash = ? WHERE id = ?
`

type SetValidatorActionOutcomeParams struct {
	Outcome string         `json:"outcome"`
	TxHash  sql.NullString `json:"tx_hash"`
	ID      int64          `json:"id"`
}

func (q *Queries) SetValidatorActionOutcome(ctx context.Context, arg SetValidatorActionOutcomeParams) error {
	_, err := q.db.ExecContext(ctx, SetValidatorActionOutcome, arg.Outcome, arg.TxHash, arg.ID)
	return err
}

const StakerExists = `-- name: StakerExists :one
SELECT EXISTS(SELECT 1 FROM stakers WHERE address = ?)
`

func (q *Queries) StakerExists(ctx context.Context, address string) (int64, error) {
	row := q.db.QueryRowContext(ctx, StakerExists, address)
	var column_1 int64
	err := row.Scan(&column_1)
	return column_1, err
}

const SumRewardsAmount = `-- name: SumRewardsAmount :one
SELECT CAST(COALESCE(SUM(amount), 0) AS INTEGER) FROM rewards
`

func (q *Queries) SumRewardsAmount(ctx context.Context) (int64, error) {
	row := q.db.QueryRowContext(ctx, SumRewardsAmount)
	var column_1 int64
	err := row.Scan(&column_1)
	return column_1, err
}
```

- [ ] **Step 9: Build and run the full test suite**

Run: `go build ./... && go test ./...`
Expected: builds clean; all existing tests plus `TestValidateAPI` pass.

- [ ] **Step 10: Document the new config fields**

In `config/config.json-example`, add after `"auto_reactivate": true`:

```json
    "api_addr": ":8080",
    "session_secret": "generate a long random string for this — used to sign login sessions",
    "validator_address": "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"
```

(the `validator_address` should be the pool's own public address — the same one `chain.Address()` derives from `private_key` — but is given here explicitly since `cmd/api` never loads the private key.)

- [ ] **Step 11: Commit**

```bash
git add internal/config internal/chain internal/pool/pool.go sql/queries.sql internal/db/queries.sql.go config/config.json-example
git commit -m "feat(config,db): API config fields, read-only RPC client, API query set"
```

---

## Task 2: Daemon-side operator action polling

**Files:**
- Modify: `internal/pool/lifecycle.go`
- Modify: `internal/pool/lifecycle_test.go`
- Modify: `internal/pool/pool.go` (wire into `Run`)
- Modify: `cmd/main.go` (update CLI call sites)

**Interfaces:**
- Consumes: `db.Queries.GetRequestedValidatorActions`, `.SetValidatorActionOutcome` (Task 1).
- Produces: `Manager.Deactivate(ctx) (string, error)`, `Manager.Retire(ctx) (string, error)` (signature change — previously `error` only), `Manager.ProcessRequestedActions(ctx context.Context) error`.

- [ ] **Step 1: Write the failing test**

Add to `internal/pool/lifecycle_test.go`:

```go
func TestActionForRequest(t *testing.T) {
	cases := []struct {
		action  string
		wantErr bool
	}{
		{"deactivate", false},
		{"retire", false},
		{"delete", true}, // delete needs a recipient/value the poll loop can't supply — operator-only via CLI
		{"bogus", true},
	}
	for _, c := range cases {
		_, err := actionForRequest(c.action)
		if (err != nil) != c.wantErr {
			t.Errorf("actionForRequest(%q) error = %v, wantErr %v", c.action, err, c.wantErr)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/pool/... -run TestActionForRequest -v`
Expected: FAIL — `actionForRequest` undefined.

- [ ] **Step 3: Refactor Deactivate/Retire and add the poll step**

In `internal/pool/lifecycle.go`, change `Deactivate` and `Retire` to return the tx hash instead of printing it (printing moves to the CLI caller in `cmd/main.go`, Step 5):

```go
// Deactivate submits a DeactivateValidator transaction for this pool.
func (m *Manager) Deactivate(ctx context.Context) (string, error) {
	addr := m.chain.Address()
	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return "", err
	}
	tx, err := nimiq.NewDeactivateValidatorTransaction(addr, addr, 0, head, m.chain.Network)
	if err != nil {
		return "", err
	}
	if err := nimiq.SignStakingTransaction(ctx, tx, m.chain.Signer, m.chain.Signer); err != nil {
		return "", err
	}
	return m.chain.RPC.SendTransaction(ctx, tx)
}

// Retire submits a RetireValidator transaction for this pool.
func (m *Manager) Retire(ctx context.Context) (string, error) {
	addr := m.chain.Address()
	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return "", err
	}
	tx, err := nimiq.NewRetireValidatorTransaction(addr, 0, head, m.chain.Network)
	if err != nil {
		return "", err
	}
	if err := nimiq.SignStakingTransaction(ctx, tx, m.chain.Signer, m.chain.Signer); err != nil {
		return "", err
	}
	return m.chain.RPC.SendTransaction(ctx, tx)
}
```

Remove the now-unused `"fmt"` import if `Delete` (unchanged, still prints) is the only remaining user of `fmt.Printf` — check before removing; `Delete` still uses it, so `fmt` stays imported.

Add, after `Retire`:

```go
// actionForRequest maps a validator_actions.action string to the Manager
// method the poll loop should call. Only deactivate/retire are pollable —
// delete needs an operator-supplied recipient and deposit value the request
// row has no place for, so it stays CLI-only (see cmd/main.go).
func actionForRequest(action string) (func(context.Context, *Manager) (string, error), error) {
	switch action {
	case "deactivate":
		return (*Manager).Deactivate, nil
	case "retire":
		return (*Manager).Retire, nil
	}
	return nil, fmt.Errorf("pool: action %q is not requestable, only deactivate/retire are", action)
}

// ProcessRequestedActions submits every validator_actions row an operator
// queued via the API (outcome = "requested"), then records the outcome.
// This is the only bridge between the API (which never holds the validator's
// key) and an actual signed transaction — the API writes the request, the
// daemon (which does hold the key) executes it.
func (m *Manager) ProcessRequestedActions(ctx context.Context) error {
	requests, err := m.queries.GetRequestedValidatorActions(ctx)
	if err != nil {
		return err
	}
	for _, req := range requests {
		fn, err := actionForRequest(req.Action)
		if err != nil {
			logger.Logger.Error("skipping unrequestable validator action", zap.String("action", req.Action), zap.Error(err))
			if setErr := m.queries.SetValidatorActionOutcome(ctx, db.SetValidatorActionOutcomeParams{
				Outcome: "failed", ID: req.ID,
			}); setErr != nil {
				return setErr
			}
			continue
		}

		hash, err := fn(ctx, m)
		outcome := "pending"
		if err != nil {
			outcome = "failed"
			logger.Logger.Error("requested validator action failed", zap.String("action", req.Action), zap.Error(err))
		} else {
			logger.Logger.Info("requested validator action submitted", zap.String("action", req.Action), zap.String("tx", hash))
		}
		if setErr := m.queries.SetValidatorActionOutcome(ctx, db.SetValidatorActionOutcomeParams{
			Outcome: outcome, TxHash: sql.NullString{String: hash, Valid: hash != ""}, ID: req.ID,
		}); setErr != nil {
			return setErr
		}
	}
	return nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/pool/... -run TestActionForRequest -v`
Expected: PASS

- [ ] **Step 5: Update the CLI call sites**

In `cmd/main.go`, `runValidatorCLI`:

```go
	case "deactivate":
		hash, err := m.Deactivate(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("deactivate transaction submitted: %s\n", hash)
		return nil
	case "retire":
		hash, err := m.Retire(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("retire transaction submitted: %s\n", hash)
		return nil
```

- [ ] **Step 6: Wire the poll step into the daemon loop**

In `internal/pool/pool.go`, `Run`, add alongside the existing `runPayouts`/`runConfirmations`/`runAutoReactivate` calls:

```go
		if err := m.ProcessRequestedActions(ctx); err != nil {
			logger.Logger.Error("processing requested validator actions", zap.Error(err))
		}
```

- [ ] **Step 7: Build and run the full suite**

Run: `go build ./... && go test ./...`
Expected: builds clean, all tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/pool cmd/main.go
git commit -m "feat(pool): poll operator-requested validator actions from the API"
```

---

## Task 3: API server skeleton + GET /api/pool

**Files:**
- Create: `internal/api/api.go`
- Create: `internal/api/pool_handlers.go`
- Create: `internal/api/testdb.go` (test-only helper, but not `_test.go` — see note below)
- Create: `internal/api/pool_handlers_test.go`
- Create: `cmd/api/main.go`

**Interfaces:**
- Consumes: `db.New`, `db.InitDB` (existing), `config.LoadConfig`, `config.ValidateAPI`, `chain.NewRPCOnly` (Task 1).
- Produces: `api.New(cfg *config.Config, q *db.Queries, rpcClient *rpc.Client) *api.API`, `(*API).Mux() http.Handler`, JSON shape for `GET /api/pool`.

A note on the test helper: `internal/pool`'s existing tests are pure-function unit tests with no DB, so there's no established sqlite-test-fixture pattern to reuse. `internal/api`'s handlers need one, and every task from here on reuses it, so it's built once now as a small non-`_test.go` file (`testdb.go`) that only test files import — this avoids repeating the schema-loading boilerplate in five different `_test.go` files.

- [ ] **Step 1: Add the shared test DB helper**

Create `internal/api/testdb.go`:

```go
package api

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/Beardsoft/GoPool/internal/db"
)

// newTestDB opens an in-memory sqlite database and applies the project
// schema, for handler tests. It locates schema/scheme.sql relative to this
// source file (not the process CWD), so it works no matter which package
// directory `go test` runs from.
func newTestDB(t *testing.T) *db.Queries {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	schemaPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "schema", "scheme.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("reading schema: %v", err)
	}

	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if _, err := sqlDB.Exec(string(schema)); err != nil {
		t.Fatalf("applying schema: %v", err)
	}
	return db.New(sqlDB)
}
```

- [ ] **Step 2: Write the failing handler test**

Create `internal/api/pool_handlers_test.go`:

```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Beardsoft/GoPool/internal/db"
)

func TestHandlePool(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	if err := q.InsertEpoch(ctx, db.InsertEpochParams{Number: 3, NumStakers: 2, Balance: 100_000_00000, Status: "in_progress"}); err != nil {
		t.Fatalf("seeding epoch: %v", err)
	}
	if err := q.InsertReward(ctx, db.InsertRewardParams{BatchNumber: 1, EpochNumber: 2, Amount: 990_00000, PoolFee: 10_00000, NumStakers: 2}); err != nil {
		t.Fatalf("seeding reward: %v", err)
	}

	a := &API{queries: q}
	req := httptest.NewRequest(http.MethodGet, "/api/pool", nil)
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	var got poolResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.CurrentEpoch != 3 || got.NumStakers != 2 || got.TotalStakeLuna != 100_000_00000 || got.TotalRewardsLuna != 990_00000 {
		t.Errorf("got %+v", got)
	}
}
```

- [ ] **Step 3: Run it to verify it fails**

Run: `go test ./internal/api/... -run TestHandlePool -v`
Expected: FAIL — package `api` and its types don't exist yet.

- [ ] **Step 4: Implement the API struct and router**

Create `internal/api/api.go`:

```go
// Package api serves GoPool's read/operator HTTP API. It never holds the
// validator's private key — see cmd/api/main.go and internal/pool's
// ProcessRequestedActions for why.
package api

import (
	"net/http"

	"github.com/NimMiniApps/nimiq-go/rpc"

	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
)

// API holds the dependencies every handler needs.
type API struct {
	queries *db.Queries
	cfg     *config.Config
	rpc     *rpc.Client
	nonces  *nonceStore
}

// New builds an API. cfg may be nil in tests that don't exercise auth or the
// operator health endpoint; rpc may be nil in tests that don't exercise
// operator health.
func New(cfg *config.Config, q *db.Queries, rpcClient *rpc.Client) *API {
	return &API{queries: q, cfg: cfg, rpc: rpcClient, nonces: newNonceStore()}
}

// Mux builds the HTTP routes. Route registration for auth/operator handlers
// is added in later tasks; this file owns the mux itself so every task edits
// only the routes it adds, not a shared switch statement.
func (a *API) Mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/pool", a.handlePool)
	a.registerEpochRoutes(mux)
	a.registerStakerRoutes(mux)
	a.registerAuthRoutes(mux)
	a.registerOperatorRoutes(mux)
	a.registerStaticRoutes(mux)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
```

Add the `encoding/json` import (`"encoding/json"`) to the import block above.

This references `registerEpochRoutes`, `registerStakerRoutes`, `registerAuthRoutes`, `registerOperatorRoutes`, and `registerStaticRoutes`, which don't exist until Tasks 4, 5, 6, 8, and 9. For this task only, stub the four not yet built as no-ops at the bottom of `api.go` (Tasks 4–9 each replace their stub with the real implementation in their own file and delete it here):

```go
func (a *API) registerEpochRoutes(mux *http.ServeMux)    {}
func (a *API) registerStakerRoutes(mux *http.ServeMux)   {}
func (a *API) registerAuthRoutes(mux *http.ServeMux)     {}
func (a *API) registerOperatorRoutes(mux *http.ServeMux) {}
func (a *API) registerStaticRoutes(mux *http.ServeMux)   {}
```

Create `internal/api/pool_handlers.go`:

```go
package api

import "net/http"

type poolResponse struct {
	CurrentEpoch      int64   `json:"current_epoch"`
	EpochStatus       string  `json:"epoch_status"`
	NumStakers        int64   `json:"num_stakers"`
	TotalStakeLuna    int64   `json:"total_stake_luna"`
	TotalRewardsLuna  int64   `json:"total_rewards_luna"`
	PoolFeePercentage float64 `json:"pool_fee_percentage"`
}

func (a *API) handlePool(w http.ResponseWriter, r *http.Request) {
	epoch, err := a.queries.GetLatestEpoch(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading pool state")
		return
	}
	totalRewards, err := a.queries.SumRewardsAmount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading pool state")
		return
	}
	feePct := 0.0
	if a.cfg != nil {
		feePct = a.cfg.PoolFeePercentage
	}
	writeJSON(w, http.StatusOK, poolResponse{
		CurrentEpoch:      epoch.Number,
		EpochStatus:       epoch.Status,
		NumStakers:        epoch.NumStakers,
		TotalStakeLuna:    epoch.Balance,
		TotalRewardsLuna:  totalRewards,
		PoolFeePercentage: feePct,
	})
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/api/... -run TestHandlePool -v`
Expected: PASS

- [ ] **Step 6: Add the API entrypoint**

Create `cmd/api/main.go`:

```go
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Beardsoft/GoPool/internal/api"
	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"

	"go.uber.org/zap"
)

func main() {
	logger.InitLogger()
	defer logger.Sync()

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Logger.Fatal("failed to load config", zap.Error(err))
	}
	if err := config.ValidateAPI(cfg); err != nil {
		logger.Logger.Fatal("invalid API config", zap.Error(err))
	}

	rpcClient, err := chain.NewRPCOnly(cfg)
	if err != nil {
		logger.Logger.Fatal("failed to set up RPC client", zap.Error(err))
	}

	sqlDB, err := db.InitDB("pool.db")
	if err != nil {
		logger.Logger.Fatal("failed to initialize the database", zap.Error(err))
	}
	defer sqlDB.Close()
	queries := db.New(sqlDB)

	a := api.New(cfg, queries, rpcClient)
	srv := &http.Server{Addr: cfg.APIAddr, Handler: a.Mux()}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logger.Logger.Info("API listening", zap.String("addr", cfg.APIAddr))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Logger.Fatal("API server stopped", zap.Error(err))
	}
}
```

- [ ] **Step 7: Build and test**

Run: `go build ./... && go test ./...`
Expected: builds clean, all tests pass (including the daemon's existing ones — this task never touched `cmd` or `internal/pool`).

- [ ] **Step 8: Commit**

```bash
git add internal/api cmd/api
git commit -m "feat(api): server skeleton and GET /api/pool"
```

---

## Task 4: GET /api/epochs, GET /api/epochs/{number}

**Files:**
- Create: `internal/api/epoch_handlers.go`
- Create: `internal/api/epoch_handlers_test.go`
- Modify: `internal/api/api.go` (remove the `registerEpochRoutes` stub)

**Interfaces:**
- Consumes: `db.Queries.ListEpochs`, `.GetEpochByNumber`, `.GetStakersForEpoch` (existing).
- Produces: JSON shapes for `GET /api/epochs` and `GET /api/epochs/{number}`.

- [ ] **Step 1: Write the failing test**

Create `internal/api/epoch_handlers_test.go`:

```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Beardsoft/GoPool/internal/db"
)

func TestHandleListEpochs(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	if err := q.InsertEpoch(ctx, db.InsertEpochParams{Number: 1, NumStakers: 1, Balance: 10, Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertEpoch(ctx, db.InsertEpochParams{Number: 2, NumStakers: 1, Balance: 20, Status: "in_progress"}); err != nil {
		t.Fatal(err)
	}

	a := &API{queries: q}
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/epochs", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got []epochResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Number != 2 { // ORDER BY number DESC
		t.Errorf("got %+v", got)
	}
}

func TestHandleGetEpoch(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	if err := q.InsertEpoch(ctx, db.InsertEpochParams{Number: 5, NumStakers: 1, Balance: 50, Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertStaker(ctx, db.InsertStakerParams{EpochNumber: 5, Address: "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E", Stake: 50, Percentage: 100}); err != nil {
		t.Fatal(err)
	}

	a := &API{queries: q}

	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/epochs/5", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got epochDetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Number != 5 || len(got.Stakers) != 1 {
		t.Errorf("got %+v", got)
	}

	rec2 := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/epochs/999", nil))
	if rec2.Code != http.StatusNotFound {
		t.Errorf("unknown epoch: status = %d, want 404", rec2.Code)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/api/... -run TestHandleListEpochs -v`
Expected: FAIL — `registerEpochRoutes` is currently a no-op stub, so both routes 404.

- [ ] **Step 3: Implement**

Create `internal/api/epoch_handlers.go`:

```go
package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/Beardsoft/GoPool/internal/db"
)

type epochResponse struct {
	Number     int64  `json:"number"`
	NumStakers int64  `json:"num_stakers"`
	BalanceLuna int64 `json:"balance_luna"`
	Status     string `json:"status"`
}

type stakerResponse struct {
	Address    string  `json:"address"`
	StakeLuna  int64   `json:"stake_luna"`
	Percentage float64 `json:"percentage"`
}

type epochDetailResponse struct {
	epochResponse
	Stakers []stakerResponse `json:"stakers"`
}

func (a *API) registerEpochRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/epochs", a.handleListEpochs)
	mux.HandleFunc("GET /api/epochs/{number}", a.handleGetEpoch)
}

func (a *API) handleListEpochs(w http.ResponseWriter, r *http.Request) {
	rows, err := a.queries.ListEpochs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading epochs")
		return
	}
	out := make([]epochResponse, len(rows))
	for i, row := range rows {
		out[i] = epochResponse{Number: row.Number, NumStakers: row.NumStakers, BalanceLuna: row.Balance, Status: row.Status}
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) handleGetEpoch(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid epoch number")
		return
	}

	epoch, err := a.queries.GetEpochByNumber(r.Context(), number)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "epoch not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading epoch")
		return
	}

	stakerRows, err := a.queries.GetStakersForEpoch(r.Context(), number)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading stakers")
		return
	}
	stakers := make([]stakerResponse, len(stakerRows))
	for i, s := range stakerRows {
		stakers[i] = stakerResponse{Address: s.Address, StakeLuna: s.Stake, Percentage: s.Percentage}
	}

	writeJSON(w, http.StatusOK, epochDetailResponse{
		epochResponse: epochResponse{Number: epoch.Number, NumStakers: epoch.NumStakers, BalanceLuna: epoch.Balance, Status: epoch.Status},
		Stakers:       stakers,
	})
}
```

In `internal/api/api.go`, delete the line `func (a *API) registerEpochRoutes(mux *http.ServeMux)    {}`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/api/... -v`
Expected: PASS for `TestHandlePool`, `TestHandleListEpochs`, `TestHandleGetEpoch`.

- [ ] **Step 5: Commit**

```bash
git add internal/api
git commit -m "feat(api): epoch list/detail endpoints"
```

---

## Task 5: GET /api/stakers/{address}

**Files:**
- Create: `internal/api/staker_handlers.go`
- Create: `internal/api/staker_handlers_test.go`
- Modify: `internal/api/api.go` (remove the `registerStakerRoutes` stub)

**Interfaces:**
- Consumes: `db.Queries.StakerExists`, `.GetStakerLatest`, `.GetPayslipsForAddress`, `.GetTransactionsForAddress` (Task 1); `nimiq.ParseAddress` (existing dependency).
- Produces: `stakerDetail(ctx, queries, address string) (stakerDetailResponse, error)` — factored out so Task 7's `GET /api/me` can reuse it instead of duplicating the query fan-out.

- [ ] **Step 1: Write the failing test**

Create `internal/api/staker_handlers_test.go`:

```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Beardsoft/GoPool/internal/db"
)

const testAddr = "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"

func TestHandleGetStaker(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	if err := q.InsertEpoch(ctx, db.InsertEpochParams{Number: 1, NumStakers: 1, Balance: 100, Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertStaker(ctx, db.InsertStakerParams{EpochNumber: 1, Address: testAddr, Stake: 100, Percentage: 100}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertPayslip(ctx, db.InsertPayslipParams{BatchNumber: 1, Address: testAddr, Amount: 5, Status: "completed"}); err != nil {
		t.Fatal(err)
	}

	a := &API{queries: q}

	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/stakers/"+testAddr, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got stakerDetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.StakeLuna != 100 || len(got.Payslips) != 1 {
		t.Errorf("got %+v", got)
	}

	rec2 := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/stakers/NQ00 0000 0000 0000 0000 0000 0000 0000 0000", nil))
	if rec2.Code != http.StatusNotFound {
		t.Errorf("unknown staker: status = %d, want 404", rec2.Code)
	}

	rec3 := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/api/stakers/not-an-address", nil))
	if rec3.Code != http.StatusBadRequest {
		t.Errorf("malformed address: status = %d, want 400", rec3.Code)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/api/... -run TestHandleGetStaker -v`
Expected: FAIL — route not implemented.

- [ ] **Step 3: Implement**

Create `internal/api/staker_handlers.go`:

```go
package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/db"
)

type payslipResponse struct {
	BatchNumber int64  `json:"batch_number"`
	AmountLuna  int64  `json:"amount_luna"`
	Status      string `json:"status"`
	TxHash      string `json:"tx_hash,omitempty"`
}

type transactionResponse struct {
	Hash       string `json:"hash"`
	AmountLuna int64  `json:"amount_luna"`
	Status     string `json:"status"`
}

type stakerDetailResponse struct {
	Address    string                `json:"address"`
	StakeLuna  int64                 `json:"stake_luna"`
	Percentage float64               `json:"percentage"`
	Payslips   []payslipResponse     `json:"payslips"`
	Transactions []transactionResponse `json:"transactions"`
}

var errStakerNotFound = errors.New("api: staker not found")

// stakerDetail fans out the reads GET /api/stakers/{address} and GET /api/me
// both need, taking a pre-parsed address so callers that already resolved
// one from a session (Task 7) don't reparse it.
func stakerDetail(ctx context.Context, q *db.Queries, addr nimiq.Address) (stakerDetailResponse, error) {
	compact := addr.Compact()

	exists, err := q.StakerExists(ctx, compact)
	if err != nil {
		return stakerDetailResponse{}, err
	}
	if exists == 0 {
		return stakerDetailResponse{}, errStakerNotFound
	}

	latest, err := q.GetStakerLatest(ctx, compact)
	if err != nil {
		return stakerDetailResponse{}, err
	}

	payslipRows, err := q.GetPayslipsForAddress(ctx, compact)
	if err != nil {
		return stakerDetailResponse{}, err
	}
	payslips := make([]payslipResponse, len(payslipRows))
	for i, p := range payslipRows {
		payslips[i] = payslipResponse{BatchNumber: p.BatchNumber, AmountLuna: p.Amount, Status: p.Status, TxHash: p.TxHash.String}
	}

	txRows, err := q.GetTransactionsForAddress(ctx, compact)
	if err != nil {
		return stakerDetailResponse{}, err
	}
	txs := make([]transactionResponse, len(txRows))
	for i, tx := range txRows {
		txs[i] = transactionResponse{Hash: tx.Hash, AmountLuna: tx.Amount, Status: tx.Status}
	}

	return stakerDetailResponse{
		Address: addr.String(), StakeLuna: latest.Stake, Percentage: latest.Percentage,
		Payslips: payslips, Transactions: txs,
	}, nil
}

func (a *API) registerStakerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/stakers/{address}", a.handleGetStaker)
}

func (a *API) handleGetStaker(w http.ResponseWriter, r *http.Request) {
	addr, err := nimiq.ParseAddress(r.PathValue("address"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid address")
		return
	}

	detail, err := stakerDetail(r.Context(), a.queries, addr)
	if errors.Is(err, errStakerNotFound) {
		writeError(w, http.StatusNotFound, "no staker with this address in this pool")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading staker")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
```

Note: `sql` is imported but unused directly here — `GetStakerLatest`/`GetPayslipsForAddress` return `sql.ErrNoRows` only via the `StakerExists` guard, which is already checked, so the `errors`/`database/sql` imports are for `errStakerNotFound` wiring only; if `go vet` flags `database/sql` as unused, remove that import — it is not needed since no `sql.` symbol is referenced directly in this file. (Only `errors` is actually used.)

In `internal/api/api.go`, delete the line `func (a *API) registerStakerRoutes(mux *http.ServeMux)   {}`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go build ./... && go test ./internal/api/... -v`
Expected: builds clean (fix the unused-import note from Step 3 if `go vet`/`go build` flags it), `TestHandleGetStaker` passes along with everything from Tasks 3–4.

- [ ] **Step 5: Commit**

```bash
git add internal/api
git commit -m "feat(api): staker detail endpoint"
```

---

## Task 6: Nimiq Hub login (challenge/verify) and session middleware

**Files:**
- Create: `internal/api/auth.go`
- Create: `internal/api/auth_test.go`
- Modify: `internal/api/api.go` (remove the `registerAuthRoutes` stub)

**Interfaces:**
- Consumes: `nimiq.ParseAddress`, `nimiq.ParsePublicKey`, `nimiq.ParseSignature`, `nimiq.VerifyMessageFrom`, `nimiq.SignMessage`/`nimiq.Signer` — only `VerifyMessageFrom` etc. are used server-side (see `nimiq-go`'s `docs/wallet-login.md`).
- Produces: `newNonceStore() *nonceStore`; `(*API).requireSession(next http.HandlerFunc) http.HandlerFunc` (sets the caller's `nimiq.Address` in the request context, 401s if absent/invalid); `addressFromContext(ctx) (nimiq.Address, bool)`.

Note on testing Hub verification without a browser: `nimiq-go`'s own `ed25519`-based signer (`signer.PrivateKey`, already used by `internal/chain`) can sign a message exactly as Hub would, via `nimiq.SignMessage`. The test below uses that to produce a real, valid signature over a real challenge, so the test exercises the actual cryptographic path — not a mock.

- [ ] **Step 1: Write the failing test**

Create `internal/api/auth_test.go`:

```go
package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/NimMiniApps/nimiq-go/signer"

	"github.com/Beardsoft/GoPool/internal/config"
)

func TestAuthChallengeVerifyRoundTrip(t *testing.T) {
	key, err := signer.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	addr := key.Address()

	a := &API{queries: newTestDB(t), cfg: &config.Config{SessionSecret: "test-secret"}, nonces: newNonceStore()}

	// 1. challenge
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/challenge", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("challenge status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var ch challengeResponse
	if err := json.NewDecoder(rec.Body).Decode(&ch); err != nil {
		t.Fatal(err)
	}

	// 2. sign it exactly as Hub's signMessage would
	ctx := context.Background()
	signed, err := nimiq.SignMessage(ctx, key, []byte(ch.Challenge))
	if err != nil {
		t.Fatal(err)
	}

	// 3. verify
	body := strings.NewReader(`{"nonce":"` + ch.Nonce + `","address":"` + addr.String() + `","public_key":"` +
		base64.StdEncoding.EncodeToString(signed.PublicKey) + `","signature":"` +
		base64.StdEncoding.EncodeToString(signed.Signature) + `"}`)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/verify", body)
	req2.Header.Set("Content-Type", "application/json")
	a.Mux().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("verify status = %d, body: %s", rec2.Code, rec2.Body.String())
	}
	cookies := rec2.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName {
		t.Fatalf("expected one %s cookie, got %+v", sessionCookieName, cookies)
	}

	// 4. reuse fails (single-use nonce)
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/api/auth/verify", strings.NewReader(`{"nonce":"`+ch.Nonce+`","address":"`+addr.String()+`","public_key":"`+
		base64.StdEncoding.EncodeToString(signed.PublicKey)+`","signature":"`+base64.StdEncoding.EncodeToString(signed.Signature)+`"}`))
	req3.Header.Set("Content-Type", "application/json")
	a.Mux().ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusBadRequest {
		t.Errorf("nonce reuse: status = %d, want 400", rec3.Code)
	}

	// 5. the session cookie authenticates requireSession
	protected := a.requireSession(func(w http.ResponseWriter, r *http.Request) {
		got, ok := addressFromContext(r.Context())
		if !ok || got != addr {
			t.Errorf("addressFromContext = %v, %v; want %v, true", got, ok, addr)
		}
		w.WriteHeader(http.StatusOK)
	})
	rec4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req4.AddCookie(cookies[0])
	protected(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Errorf("requireSession: status = %d, want 200", rec4.Code)
	}
}

func TestRequireSessionRejectsMissingCookie(t *testing.T) {
	a := &API{cfg: &config.Config{SessionSecret: "test-secret"}}
	protected := a.requireSession(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	rec := httptest.NewRecorder()
	protected(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
```

Check that `signer.GeneratePrivateKey` and `signer.PrivateKey.Address()` exist before writing this — grep `/home/maestro/go/pkg/mod/github.com/!Nim!Mini!Apps/nimiq-go@v0.4.0/signer/*.go` for `func Generate` and `func (k *PrivateKey) Address`. If the real names differ, use those instead (the test's job is a real round trip, not these exact names).

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/api/... -run TestAuth -v`
Expected: FAIL — `auth.go` doesn't exist yet.

- [ ] **Step 3: Implement**

Create `internal/api/auth.go`:

```go
package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	nimiq "github.com/NimMiniApps/nimiq-go"
)

const (
	sessionCookieName = "gopool_session"
	sessionTTL         = 24 * time.Hour
	nonceTTL           = 5 * time.Minute
)

// nonceStore holds outstanding login challenges. In-memory and single-use:
// fine for one API process; if this ever runs behind a load balancer with
// multiple API instances, move it to the shared sqlite file instead.
type nonceStore struct {
	mu   sync.Mutex
	data map[string]nonceEntry
}

type nonceEntry struct {
	challenge string
	expires   time.Time
}

func newNonceStore() *nonceStore {
	return &nonceStore{data: make(map[string]nonceEntry)}
}

func (s *nonceStore) issue() (nonce, challenge string) {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	nonce = base64.RawURLEncoding.EncodeToString(buf[:])
	challenge = fmt.Sprintf("GoPool login\nnonce: %s\nexpires: %s", nonce, time.Now().Add(nonceTTL).Format(time.RFC3339))

	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[nonce] = nonceEntry{challenge: challenge, expires: time.Now().Add(nonceTTL)}
	return nonce, challenge
}

// consume returns the challenge text for nonce and deletes it — single use,
// whether or not verification against it later succeeds.
func (s *nonceStore) consume(nonce string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.data[nonce]
	delete(s.data, nonce)
	if !ok || time.Now().After(entry.expires) {
		return "", false
	}
	return entry.challenge, true
}

type challengeResponse struct {
	Nonce     string `json:"nonce"`
	Challenge string `json:"challenge"`
}

type verifyRequest struct {
	Nonce     string `json:"nonce"`
	Address   string `json:"address"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

func (a *API) registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/challenge", a.handleAuthChallenge)
	mux.HandleFunc("POST /api/auth/verify", a.handleAuthVerify)
}

func (a *API) handleAuthChallenge(w http.ResponseWriter, r *http.Request) {
	nonce, challenge := a.nonces.issue()
	writeJSON(w, http.StatusOK, challengeResponse{Nonce: nonce, Challenge: challenge})
}

func (a *API) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	challenge, ok := a.nonces.consume(req.Nonce)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown, expired, or already-used nonce")
		return
	}

	addr, err := nimiq.ParseAddress(req.Address)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid address")
		return
	}
	pub, err := nimiq.ParsePublicKey(req.PublicKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid public key")
		return
	}
	sig, err := nimiq.ParseSignature(req.Signature)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid signature")
		return
	}
	if err := nimiq.VerifyMessageFrom(addr, pub, []byte(challenge), sig); err != nil {
		writeError(w, http.StatusUnauthorized, "signature verification failed")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: a.issueSession(addr), Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: true, MaxAge: int(sessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]string{"address": addr.String()})
}

// issueSession builds an HMAC-signed, self-contained session token:
// address.expiry.signature. No server-side session store — verifying is
// just recomputing the HMAC, which is why SessionSecret must be kept private
// and rotating it invalidates every outstanding session at once.
func (a *API) issueSession(addr nimiq.Address) string {
	exp := time.Now().Add(sessionTTL).Unix()
	payload := addr.Compact() + "." + strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, []byte(a.cfg.SessionSecret))
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *API) parseSession(token string) (nimiq.Address, error) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return nimiq.Address{}, fmt.Errorf("api: malformed session token")
	}
	payload := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(a.cfg.SessionSecret))
	mac.Write([]byte(payload))
	want := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(want, got) {
		return nimiq.Address{}, fmt.Errorf("api: invalid session signature")
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return nimiq.Address{}, fmt.Errorf("api: session expired")
	}
	return nimiq.ParseAddress(parts[0])
}

type contextKey int

const addressContextKey contextKey = 0

func addressFromContext(ctx context.Context) (nimiq.Address, bool) {
	addr, ok := ctx.Value(addressContextKey).(nimiq.Address)
	return addr, ok
}

// requireSession 401s unless the request carries a valid, unexpired session
// cookie, and makes the authenticated address available to next via context.
func (a *API) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not logged in")
			return
		}
		addr, err := a.parseSession(cookie.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "session invalid or expired")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), addressContextKey, addr)))
	}
}
```

In `internal/api/api.go`, delete the line `func (a *API) registerAuthRoutes(mux *http.ServeMux)     {}`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go build ./... && go test ./internal/api/... -v`
Expected: PASS for `TestAuthChallengeVerifyRoundTrip`, `TestRequireSessionRejectsMissingCookie`, and everything from Tasks 3–5.

- [ ] **Step 5: Commit**

```bash
git add internal/api
git commit -m "feat(api): Nimiq Hub login and session middleware"
```

---

## Task 7: GET /api/me

**Files:**
- Modify: `internal/api/staker_handlers.go` (add the handler + route)
- Modify: `internal/api/staker_handlers_test.go`

**Interfaces:**
- Consumes: `stakerDetail` (Task 5), `(*API).requireSession`, `addressFromContext` (Task 6).

- [ ] **Step 1: Write the failing test**

Add to `internal/api/staker_handlers_test.go`:

```go
func TestHandleMe(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	if err := q.InsertEpoch(ctx, db.InsertEpochParams{Number: 1, NumStakers: 1, Balance: 100, Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertStaker(ctx, db.InsertStakerParams{EpochNumber: 1, Address: testAddr, Stake: 100, Percentage: 100}); err != nil {
		t.Fatal(err)
	}

	a := &API{queries: q, cfg: &config.Config{SessionSecret: "test-secret"}}
	addr, err := nimiq.ParseAddress(testAddr)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: a.issueSession(addr)})
	a.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got stakerDetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.StakeLuna != 100 {
		t.Errorf("got %+v", got)
	}

	rec2 := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("no cookie: status = %d, want 401", rec2.Code)
	}
}
```

Add the needed imports to `internal/api/staker_handlers_test.go`: `nimiq "github.com/NimMiniApps/nimiq-go"` and `"github.com/Beardsoft/GoPool/internal/config"`.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/api/... -run TestHandleMe -v`
Expected: FAIL — `/api/me` not routed.

- [ ] **Step 3: Implement**

In `internal/api/staker_handlers.go`, add to `registerStakerRoutes`:

```go
	mux.HandleFunc("GET /api/me", a.requireSession(a.handleMe))
```

Add the handler:

```go
func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	addr, ok := addressFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	detail, err := stakerDetail(r.Context(), a.queries, addr)
	if errors.Is(err, errStakerNotFound) {
		writeError(w, http.StatusNotFound, "this address has never staked with this pool")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading staker")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/api/... -v`
Expected: all tests from Tasks 3–7 pass.

- [ ] **Step 5: Commit**

```bash
git add internal/api
git commit -m "feat(api): GET /api/me"
```

---

## Task 8: Operator endpoints (health, deactivate, retire)

**Files:**
- Create: `internal/api/operator_handlers.go`
- Create: `internal/api/operator_handlers_test.go`
- Modify: `internal/api/api.go` (remove the `registerOperatorRoutes` stub)

**Interfaces:**
- Consumes: `(*API).requireSession` (Task 6); `db.Queries.GetCursor`, `pool.CursorName` (Task 1), `.GetStuckPayslips`, `.HasOutstandingValidatorAction`, `.InsertValidatorAction` (existing/Task 1); `a.rpc.BlockNumber`.
- Produces: `(*API).requireOperator(next http.HandlerFunc) http.HandlerFunc` — wraps `requireSession`, additionally 403s unless the session address equals `cfg.ValidatorAddress`.

- [ ] **Step 1: Write the failing test**

Create `internal/api/operator_handlers_test.go`:

```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/pool"
)

func operatorTestAPI(t *testing.T) (*API, *http.Cookie, *http.Cookie) {
	t.Helper()
	q := newTestDB(t)
	cfg := &config.Config{SessionSecret: "test-secret", ValidatorAddress: testAddr}
	a := &API{queries: q, cfg: cfg}

	operatorAddr, err := nimiq.ParseAddress(testAddr)
	if err != nil {
		t.Fatal(err)
	}
	stakerAddr, err := nimiq.ParseAddress("NQ00 0000 0000 0000 0000 0000 0000 0000 0001")
	if err != nil {
		t.Fatal(err)
	}
	operatorCookie := &http.Cookie{Name: sessionCookieName, Value: a.issueSession(operatorAddr)}
	stakerCookie := &http.Cookie{Name: sessionCookieName, Value: a.issueSession(stakerAddr)}
	return a, operatorCookie, stakerCookie
}

func TestRequireOperatorRejectsNonOperator(t *testing.T) {
	a, _, stakerCookie := operatorTestAPI(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/health", nil)
	req.AddCookie(stakerCookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestHandleOperatorDeactivate(t *testing.T) {
	a, operatorCookie, _ := operatorTestAPI(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/operator/validator/deactivate", nil)
	req.AddCookie(operatorCookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}

	rows, err := a.queries.GetRequestedValidatorActions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Action != "deactivate" {
		t.Fatalf("got %+v", rows)
	}

	// A second request while one is outstanding is rejected, not duplicated.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/operator/validator/deactivate", nil)
	req2.AddCookie(operatorCookie)
	a.Mux().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Errorf("duplicate request: status = %d, want 409", rec2.Code)
	}
}

func TestHandleOperatorHealth(t *testing.T) {
	a, operatorCookie, _ := operatorTestAPI(t)
	ctx := context.Background()
	if err := a.queries.UpsertCursor(ctx, db.UpsertCursorParams{Name: pool.CursorName, Height: 100}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/health", nil)
	req.AddCookie(operatorCookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.LastProcessedHeight != 100 {
		t.Errorf("got %+v", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/api/... -run TestHandleOperator -v`
Expected: FAIL — package doesn't compile (`healthResponse`, `requireOperator`, routes missing).

- [ ] **Step 3: Implement**

Create `internal/api/operator_handlers.go`:

```go
package api

import (
	"database/sql"
	"net/http"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/pool"
)

// requireOperator wraps requireSession: only the address configured as
// cfg.ValidatorAddress may pass. There is no separate operator secret — this
// is "controls the validator's private key," the same identity the daemon
// itself already uses to sign transactions.
func (a *API) requireOperator(next http.HandlerFunc) http.HandlerFunc {
	return a.requireSession(func(w http.ResponseWriter, r *http.Request) {
		addr, ok := addressFromContext(r.Context())
		if !ok || addr.String() != a.cfg.ValidatorAddress {
			writeError(w, http.StatusForbidden, "operator only")
			return
		}
		next(w, r)
	})
}

type stuckPayslipResponse struct {
	BatchNumber int64  `json:"batch_number"`
	Address     string `json:"address"`
	AmountLuna  int64  `json:"amount_luna"`
	Status      string `json:"status"`
	TxHash      string `json:"tx_hash,omitempty"`
}

type healthResponse struct {
	LastProcessedHeight int64                   `json:"last_processed_height"`
	ChainHead           uint32                  `json:"chain_head"`
	StuckPayslips       []stuckPayslipResponse  `json:"stuck_payslips"`
}

func (a *API) registerOperatorRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/operator/health", a.requireOperator(a.handleOperatorHealth))
	mux.HandleFunc("POST /api/operator/validator/deactivate", a.requireOperator(a.handleOperatorAction("deactivate")))
	mux.HandleFunc("POST /api/operator/validator/retire", a.requireOperator(a.handleOperatorAction("retire")))
}

func (a *API) handleOperatorHealth(w http.ResponseWriter, r *http.Request) {
	cursor, err := a.queries.GetCursor(r.Context(), pool.CursorName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading cursor")
		return
	}
	var head uint32
	if a.rpc != nil {
		head, err = a.rpc.BlockNumber(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, "querying chain head")
			return
		}
	}
	stuckRows, err := a.queries.GetStuckPayslips(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading stuck payslips")
		return
	}
	stuck := make([]stuckPayslipResponse, len(stuckRows))
	for i, s := range stuckRows {
		stuck[i] = stuckPayslipResponse{BatchNumber: s.BatchNumber, Address: s.Address, AmountLuna: s.Amount, Status: s.Status, TxHash: s.TxHash.String}
	}
	writeJSON(w, http.StatusOK, healthResponse{LastProcessedHeight: cursor, ChainHead: head, StuckPayslips: stuck})
}

// handleOperatorAction returns a handler that queues a deactivate/retire
// request for the daemon's ProcessRequestedActions to pick up and sign — the
// API itself never touches the validator's private key.
func (a *API) handleOperatorAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		outstanding, err := a.queries.HasOutstandingValidatorAction(r.Context(), action)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "checking outstanding actions")
			return
		}
		if outstanding != 0 {
			writeError(w, http.StatusConflict, action+" is already requested or in flight")
			return
		}
		if err := a.queries.InsertValidatorAction(r.Context(), db.InsertValidatorActionParams{
			Action: action, TxHash: sql.NullString{}, Outcome: "requested",
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "queuing "+action)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "requested"})
	}
}
```

In `internal/api/api.go`, delete the line `func (a *API) registerOperatorRoutes(mux *http.ServeMux) {}`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go build ./... && go test ./...`
Expected: builds clean; every test across `internal/api`, `internal/pool`, `internal/config` passes.

- [ ] **Step 5: Commit**

```bash
git add internal/api
git commit -m "feat(api): operator health and deactivate/retire request endpoints"
```

---

## Task 9: Frontend scaffold + go:embed serving

**Files:**
- Create: `web/package.json`, `web/vite.config.ts`, `web/tsconfig.json`, `web/index.html`
- Create: `web/src/main.ts`, `web/src/App.vue`, `web/src/router.ts`, `web/src/api.ts`, `web/src/style.css`
- Create: `internal/api/embed.go`
- Modify: `internal/api/api.go` (remove the `registerStaticRoutes` stub)

**Interfaces:**
- Produces: `web/dist/` (Vite build output, embedded); the frontend's `api.ts` fetch wrapper other pages import from (`apiGet<T>(path): Promise<T>`, `apiPost<T>(path, body?): Promise<T>`).

- [ ] **Step 1: Scaffold the Vite project**

Create `web/package.json`:

```json
{
  "name": "gopool-web",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vue-tsc -b && vite build"
  },
  "dependencies": {
    "@nimiq/hub-api": "^1.4.0",
    "vue": "^3.5.13",
    "vue-router": "^4.5.0"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "^5.2.1",
    "typescript": "^5.7.2",
    "vite": "^6.0.5",
    "vue-tsc": "^2.2.0"
  }
}
```

Create `web/vite.config.ts`:

```ts
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  build: { outDir: 'dist' },
  server: {
    proxy: { '/api': 'http://localhost:8080' },
  },
})
```

Create `web/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "strict": true,
    "jsx": "preserve",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "skipLibCheck": true
  },
  "include": ["src"]
}
```

Create `web/index.html`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>GoPool</title>
  </head>
  <body>
    <div id="app"></div>
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>
```

- [ ] **Step 2: Add the fetch wrapper**

Create `web/src/api.ts`:

```ts
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, { credentials: 'include', ...init })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error ?? `request to ${path} failed with ${res.status}`)
  }
  return res.json() as Promise<T>
}

export function apiGet<T>(path: string): Promise<T> {
  return request<T>(path)
}

export function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
}
```

- [ ] **Step 3: Add the router, app shell, and entrypoint**

Create `web/src/router.ts` (routes reference page components created in Tasks 10–11; this file is the seam every later task adds to):

```ts
import { createRouter, createWebHistory } from 'vue-router'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'pool', component: () => import('./pages/PoolOverview.vue') },
    { path: '/epochs', name: 'epochs', component: () => import('./pages/Epochs.vue') },
    { path: '/epochs/:number', name: 'epoch-detail', component: () => import('./pages/EpochDetail.vue'), props: true },
    { path: '/stakers/:address?', name: 'staker-lookup', component: () => import('./pages/StakerLookup.vue'), props: true },
    { path: '/me', name: 'my-dashboard', component: () => import('./pages/MyDashboard.vue') },
    { path: '/operator', name: 'operator', component: () => import('./pages/Operator.vue') },
  ],
})
```

Create `web/src/App.vue`:

```vue
<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
</script>

<template>
  <header class="nav">
    <RouterLink to="/">Pool</RouterLink>
    <RouterLink to="/epochs">Epochs</RouterLink>
    <RouterLink to="/stakers">Look up a staker</RouterLink>
    <RouterLink to="/me">My dashboard</RouterLink>
    <RouterLink to="/operator">Operator</RouterLink>
  </header>
  <main>
    <RouterView />
  </main>
</template>

<style scoped>
.nav {
  display: flex;
  gap: var(--space-16, 16px);
  padding: var(--space-16, 16px);
  font-family: 'Muli', sans-serif;
}
</style>
```

Create `web/src/main.ts`:

```ts
import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import './style.css'

createApp(App).use(router).mount('#app')
```

Create `web/src/style.css` — import the Nimiq UI Kit token set and set the base font:

```css
@import url('https://fonts.googleapis.com/css2?family=Muli:wght@400;600;700&family=Fira+Mono&display=swap');

:root {
  --nimiq-blue: #1f2348;
  --nimiq-light-blue: #0582ca;
  --nimiq-green: #21bca5;
  --nimiq-gold: #e9b213;
  --nimiq-red: #d94432;
  --text-100: rgba(31, 35, 72, 1);
  --text-80: rgba(31, 35, 72, 0.8);
  --text-60: rgba(31, 35, 72, 0.6);
  --text-40: rgba(31, 35, 72, 0.4);
  --space-16: 16px;
}

html {
  font-size: 8px;
}

body {
  font-family: 'Muli', sans-serif;
  color: var(--text-100);
  margin: 0;
}

code, .address, .hash {
  font-family: 'Fira Mono', monospace;
}
```

(This is a minimal hand-picked subset of the kit's token values, not the full `tokens.css` file — copy the complete set from `/home/maestro/Documents/projects/nimiq-ui-kit/tokens/tokens.css` if deeper kit alignment is wanted later; see the `nimiq-ui-kit` skill.)

- [ ] **Step 4: Add placeholder page components**

Tasks 10–11 replace these; for this task, create the minimum each route needs to exist so the router doesn't 404 and `vue-tsc` type-checks cleanly. Create `web/src/pages/PoolOverview.vue`, `Epochs.vue`, `EpochDetail.vue`, `StakerLookup.vue`, `MyDashboard.vue`, `Operator.vue`, each with this shape (swap the visible text per file):

```vue
<template>
  <p>Pool overview — implemented in Task 10.</p>
</template>
```

- [ ] **Step 5: Build the frontend**

Run: `cd web && npm install && npm run build`
Expected: `web/dist/index.html` and hashed JS/CSS assets exist.

- [ ] **Step 6: Wire go:embed**

Create `internal/api/embed.go`:

```go
package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:../../web/dist
var webDistRaw embed.FS

// webDist strips the "../../web/dist" embed prefix so the file server sees
// paths the way a browser requests them ("/", "/assets/...").
var webDist = mustSub(webDistRaw, "../../web/dist")

func mustSub(f embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

// registerStaticRoutes serves the built Vue SPA for every path go:embed
// doesn't recognize as /api/*. It falls back to index.html for unknown
// paths so client-side routes (e.g. /epochs/5, a full page load) resolve
// instead of 404ing — vue-router then takes over in the browser.
func (a *API) registerStaticRoutes(mux *http.ServeMux) {
	fileServer := http.FileServerFS(webDist)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(webDist, strings_TrimPrefixSlash(r.URL.Path)); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}

func strings_TrimPrefixSlash(p string) string {
	if len(p) > 0 && p[0] == '/' {
		return p[1:]
	}
	return p
}
```

Note: `//go:embed` paths must be relative to the file containing the directive and cannot use `..` to escape the module — Go's `go:embed` explicitly forbids `../` in the pattern. Since `internal/api/embed.go` cannot reach `web/dist` two directories up with `../../`, move `web/dist` into reach instead: **either**
(a) place `internal/api/embed.go`'s directive as `//go:embed all:web` and symlink/copy `web/dist` to `internal/api/web/dist` as a build step, **or**
(b) simpler — set Vite's build `outDir` to `internal/api/webdist` directly (update `web/vite.config.ts`'s `build.outDir` to `'../internal/api/webdist'`) and embed that:

```go
//go:embed all:webdist
var webDistRaw embed.FS
var webDist = mustSub(webDistRaw, "webdist")
```

Use option (b) — go back and change `web/vite.config.ts`'s `build: { outDir: 'dist' }` to `build: { outDir: '../internal/api/webdist' }`, rebuild (`cd web && npm run build`), and add `internal/api/webdist/` to `.gitignore` (it's a build artifact, regenerated by `npm run build` before every `go build`).

In `internal/api/api.go`, delete the line `func (a *API) registerStaticRoutes(mux *http.ServeMux)   {}`.

- [ ] **Step 7: Build and test**

Run: `cd web && npm run build && cd .. && go build ./... && go test ./...`
Expected: builds clean, all tests pass. Manually verify: `go run ./cmd/api` (with a valid `config.json`), then `curl localhost:8080/` returns the built `index.html`, and `curl localhost:8080/api/pool` returns JSON.

- [ ] **Step 8: Commit**

```bash
git add web internal/api/embed.go internal/api/api.go .gitignore
git commit -m "feat(web): Vite/Vue scaffold embedded into the API binary"
```

---

## Task 10: Pool overview and epoch pages

**Files:**
- Modify: `web/src/pages/PoolOverview.vue`
- Modify: `web/src/pages/Epochs.vue`
- Modify: `web/src/pages/EpochDetail.vue`

**Interfaces:**
- Consumes: `apiGet` (Task 9), `GET /api/pool`, `GET /api/epochs`, `GET /api/epochs/{number}` (Tasks 3–4) response shapes (`poolResponse`, `epochResponse`, `epochDetailResponse` — mirror their JSON field names in TypeScript).

- [ ] **Step 1: Pool overview**

Replace `web/src/pages/PoolOverview.vue`:

```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiGet } from '../api'

interface PoolStatus {
  current_epoch: number
  epoch_status: string
  num_stakers: number
  total_stake_luna: number
  total_rewards_luna: number
  pool_fee_percentage: number
}

const pool = ref<PoolStatus | null>(null)
const error = ref('')

function nim(luna: number): string {
  return (luna / 100000).toLocaleString(undefined, { maximumFractionDigits: 5 })
}

onMounted(async () => {
  try {
    pool.value = await apiGet<PoolStatus>('/api/pool')
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <section v-if="pool">
    <h1>Pool status</h1>
    <dl>
      <dt>Epoch</dt><dd>{{ pool.current_epoch }} ({{ pool.epoch_status }})</dd>
      <dt>Stakers</dt><dd>{{ pool.num_stakers }}</dd>
      <dt>Total delegated stake</dt><dd>{{ nim(pool.total_stake_luna) }} NIM</dd>
      <dt>Cumulative rewards paid</dt><dd>{{ nim(pool.total_rewards_luna) }} NIM</dd>
      <dt>Pool fee</dt><dd>{{ (pool.pool_fee_percentage * 100).toFixed(2) }}%</dd>
    </dl>
  </section>
  <p v-else-if="error" class="error">{{ error }}</p>
  <p v-else>Loading…</p>
</template>
```

- [ ] **Step 2: Epoch list**

Replace `web/src/pages/Epochs.vue`:

```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { apiGet } from '../api'

interface Epoch {
  number: number
  num_stakers: number
  balance_luna: number
  status: string
}

const epochs = ref<Epoch[]>([])
const error = ref('')

onMounted(async () => {
  try {
    epochs.value = await apiGet<Epoch[]>('/api/epochs')
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <h1>Epochs</h1>
  <p v-if="error" class="error">{{ error }}</p>
  <table v-else>
    <thead><tr><th>Epoch</th><th>Status</th><th>Stakers</th></tr></thead>
    <tbody>
      <tr v-for="e in epochs" :key="e.number">
        <td><RouterLink :to="`/epochs/${e.number}`">{{ e.number }}</RouterLink></td>
        <td>{{ e.status }}</td>
        <td>{{ e.num_stakers }}</td>
      </tr>
    </tbody>
  </table>
</template>
```

- [ ] **Step 3: Epoch detail**

Replace `web/src/pages/EpochDetail.vue`:

```vue
<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { apiGet } from '../api'

const props = defineProps<{ number: string }>()

interface Staker {
  address: string
  stake_luna: number
  percentage: number
}
interface EpochDetail {
  number: number
  status: string
  num_stakers: number
  balance_luna: number
  stakers: Staker[]
}

const epoch = ref<EpochDetail | null>(null)
const error = ref('')

async function load() {
  error.value = ''
  epoch.value = null
  try {
    epoch.value = await apiGet<EpochDetail>(`/api/epochs/${props.number}`)
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(load)
watch(() => props.number, load)
</script>

<template>
  <p v-if="error" class="error">{{ error }}</p>
  <section v-else-if="epoch">
    <h1>Epoch {{ epoch.number }}</h1>
    <p>{{ epoch.status }} — {{ epoch.num_stakers }} stakers</p>
    <table>
      <thead><tr><th>Address</th><th>Stake (luna)</th><th>%</th></tr></thead>
      <tbody>
        <tr v-for="s in epoch.stakers" :key="s.address">
          <td class="address">{{ s.address }}</td>
          <td>{{ s.stake_luna }}</td>
          <td>{{ s.percentage.toFixed(2) }}%</td>
        </tr>
      </tbody>
    </table>
  </section>
</template>
```

- [ ] **Step 4: Build and manually verify**

Run: `cd web && npm run build && cd .. && go build ./...`

Manual check (dev server, faster than a full embed rebuild loop): `cd web && npm run dev`, with `go run ./cmd/api` running separately so the Vite proxy (configured in Task 9) reaches it. Open `http://localhost:5173/`, confirm the pool overview renders real numbers, click through to Epochs and an epoch detail page.

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "feat(web): pool overview and epoch pages"
```

---

## Task 11: Staker lookup, Hub login, and operator panel

**Files:**
- Modify: `web/src/pages/StakerLookup.vue`
- Modify: `web/src/pages/MyDashboard.vue`
- Modify: `web/src/pages/Operator.vue`
- Create: `web/src/hub.ts`

**Interfaces:**
- Consumes: `apiGet`/`apiPost` (Task 9); `GET /api/stakers/{address}`, `POST /api/auth/challenge`, `POST /api/auth/verify`, `GET /api/me`, `GET /api/operator/health`, `POST /api/operator/validator/{deactivate,retire}` (Tasks 5–8); `@nimiq/hub-api`'s `HubApi.signMessage`.

- [ ] **Step 1: Hub login helper**

Create `web/src/hub.ts` — wraps the challenge/verify round trip described in `nimiq-go`'s `docs/wallet-login.md`, client side:

```ts
import HubApi from '@nimiq/hub-api'
import { apiGet, apiPost } from './api'

// Hub's endpoint per network; adjust to the pool's configured network.
const hub = new HubApi('https://hub.nimiq.com')

interface Challenge {
  nonce: string
  challenge: string
}

export async function loginWithHub(): Promise<{ address: string }> {
  const { nonce, challenge } = await apiGet<Challenge>('/api/auth/challenge')

  const signed = await hub.signMessage({
    appName: 'GoPool',
    message: challenge,
  })

  return apiPost<{ address: string }>('/api/auth/verify', {
    nonce,
    address: signed.signerAddress,
    public_key: signed.signer,
    signature: signed.signature,
  })
}
```

Note: confirm `HubApi.signMessage`'s actual result field names (`signerAddress`/`signer`/`signature` above are the commonly documented Hub API shape) against the installed `@nimiq/hub-api` package's TypeScript types (`node_modules/@nimiq/hub-api/dist/*.d.ts`) once `npm install` has run, and adjust the field names in the `apiPost` call to match exactly — the server only cares that whatever comes back is decodable by `nimiq.ParsePublicKey`/`ParseSignature` (std base64, base64url, or hex, per Task 6), so no server change is needed if Hub's actual encoding differs from base64.

- [ ] **Step 2: Staker lookup page**

Replace `web/src/pages/StakerLookup.vue`:

```vue
<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { apiGet } from '../api'

const props = defineProps<{ address?: string }>()
const router = useRouter()
const input = ref(props.address ?? '')
const error = ref('')

interface StakerDetail {
  address: string
  stake_luna: number
  percentage: number
  payslips: { batch_number: number; amount_luna: number; status: string }[]
  transactions: { hash: string; amount_luna: number; status: string }[]
}
const staker = ref<StakerDetail | null>(null)

async function lookup(address: string) {
  error.value = ''
  staker.value = null
  if (!address) return
  try {
    staker.value = await apiGet<StakerDetail>(`/api/stakers/${encodeURIComponent(address)}`)
  } catch (e) {
    error.value = (e as Error).message
  }
}

function submit() {
  router.push(`/stakers/${encodeURIComponent(input.value)}`)
}

watch(() => props.address, (a) => { if (a) lookup(a) }, { immediate: true })
</script>

<template>
  <h1>Look up a staker</h1>
  <form @submit.prevent="submit">
    <input v-model="input" placeholder="NQ.. .... .... .... .... .... .... .... ...." class="address" />
    <button type="submit">Look up</button>
  </form>
  <p v-if="error" class="error">{{ error }}</p>
  <section v-else-if="staker">
    <p>Stake: {{ staker.stake_luna }} luna ({{ staker.percentage.toFixed(2) }}%)</p>
    <h2>Payouts</h2>
    <ul>
      <li v-for="p in staker.payslips" :key="p.batch_number">
        batch {{ p.batch_number }}: {{ p.amount_luna }} luna — {{ p.status }}
      </li>
    </ul>
  </section>
</template>
```

- [ ] **Step 3: My dashboard page**

Replace `web/src/pages/MyDashboard.vue`:

```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiGet } from '../api'
import { loginWithHub } from '../hub'

interface StakerDetail {
  address: string
  stake_luna: number
  percentage: number
  payslips: { batch_number: number; amount_luna: number; status: string }[]
}

const me = ref<StakerDetail | null>(null)
const loggedIn = ref(false)
const error = ref('')

async function load() {
  try {
    me.value = await apiGet<StakerDetail>('/api/me')
    loggedIn.value = true
  } catch {
    loggedIn.value = false
  }
}

async function login() {
  error.value = ''
  try {
    await loginWithHub()
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(load)
</script>

<template>
  <h1>My dashboard</h1>
  <button v-if="!loggedIn" @click="login">Log in with Nimiq Hub</button>
  <p v-if="error" class="error">{{ error }}</p>
  <section v-if="me">
    <p>Address: <span class="address">{{ me.address }}</span></p>
    <p>Stake: {{ me.stake_luna }} luna ({{ me.percentage.toFixed(2) }}%)</p>
    <ul>
      <li v-for="p in me.payslips" :key="p.batch_number">
        batch {{ p.batch_number }}: {{ p.amount_luna }} luna — {{ p.status }}
      </li>
    </ul>
  </section>
</template>
```

- [ ] **Step 4: Operator panel page**

Replace `web/src/pages/Operator.vue`:

```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiGet, apiPost } from '../api'
import { loginWithHub } from '../hub'

interface Health {
  last_processed_height: number
  chain_head: number
  stuck_payslips: { batch_number: number; address: string; amount_luna: number; status: string }[]
}

const health = ref<Health | null>(null)
const authorized = ref(false)
const error = ref('')
const actionStatus = ref('')

async function loadHealth() {
  try {
    health.value = await apiGet<Health>('/api/operator/health')
    authorized.value = true
  } catch {
    authorized.value = false
  }
}

async function login() {
  error.value = ''
  try {
    await loginWithHub()
    await loadHealth()
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function trigger(action: 'deactivate' | 'retire') {
  actionStatus.value = ''
  error.value = ''
  try {
    await apiPost(`/api/operator/validator/${action}`)
    actionStatus.value = `${action} requested — the daemon will submit it on its next tick.`
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(loadHealth)
</script>

<template>
  <h1>Operator panel</h1>
  <button v-if="!authorized" @click="login">Log in with Nimiq Hub</button>
  <p v-if="error" class="error">{{ error }}</p>
  <p v-if="actionStatus">{{ actionStatus }}</p>
  <section v-if="health">
    <p>Daemon height: {{ health.last_processed_height }} / chain head: {{ health.chain_head }}
      ({{ health.chain_head - health.last_processed_height }} behind)</p>
    <h2>Stuck payslips</h2>
    <ul v-if="health.stuck_payslips.length">
      <li v-for="p in health.stuck_payslips" :key="p.address + p.batch_number">
        {{ p.address }} — batch {{ p.batch_number }}, {{ p.amount_luna }} luna, {{ p.status }}
      </li>
    </ul>
    <p v-else>None.</p>
    <h2>Validator actions</h2>
    <button @click="trigger('deactivate')">Deactivate</button>
    <button @click="trigger('retire')">Retire</button>
  </section>
</template>
```

- [ ] **Step 5: Install the Hub API dependency and build**

Run: `cd web && npm install && npm run build && cd .. && go build ./...`
Expected: builds clean. If `@nimiq/hub-api`'s actual exported shape differs from the `HubApi` default-export assumption in `hub.ts`, fix the import to match what `npm install` actually resolved (check `node_modules/@nimiq/hub-api/package.json`'s `main`/`types` fields) — this is exactly the kind of detail Step 1's note flags as needing a live check.

- [ ] **Step 6: Manual verification**

`go run ./cmd/api` with a real `config.json` (RPC pointed at a devnet/testnet is enough, no need for the daemon to be running for the public pages — but the operator/deactivate flow needs the daemon running too, to actually pick up the request). Walk through: staker lookup with a known devnet address, Hub login on `/me` and `/operator` (Hub's popup requires a real Hub environment — dev-albatross/test-albatross Hub URLs may differ from `https://hub.nimiq.com` in `hub.ts`; adjust to the network-appropriate Hub endpoint before testing), and confirm a deactivate request shows up as a `validator_actions` row (`sqlite3 pool.db "select * from validator_actions"`) and that the running daemon's log shows "requested validator action submitted" on its next tick.

- [ ] **Step 7: Commit**

```bash
git add web
git commit -m "feat(web): staker lookup, Hub login, and operator panel"
```

---

## Self-Review Notes

- **Spec coverage:** every spec section has a task — architecture/process shape (Tasks 1, 3, 9), auth (Task 6), public endpoints (Tasks 3–5), session endpoint (Task 7), operator endpoints + action flow (Tasks 2, 8), data model (Task 1, reusing `outcome` instead of adding a `status` column — a simplification over the spec's literal wording, same effect, no migration needed), frontend pages (Tasks 9–11), testing (a table-driven or round-trip test in every task).
- **Deviation from the spec worth flagging explicitly:** the spec's "Data model changes" section proposed adding `validator_actions.status`. This plan reuses the existing `outcome` column with a new `"requested"` value instead, because `outcome` already serves exactly that role for `auto_reactivate` (`"pending"`/`"failed"`) — adding a second, parallel status column would leave two overlapping state fields on the same row. Functionally equivalent to the spec's intent (API writes a request, daemon executes and records the result); simpler schema.
- **Type consistency:** `Manager.Deactivate`/`Retire`'s new `(string, error)` signature (Task 2) is used consistently in `cmd/main.go` (Task 2) and `internal/pool/lifecycle.go`'s `actionForRequest` (Task 2) — no other task calls them directly. `stakerDetail`'s signature (Task 5) is reused verbatim by `handleMe` (Task 7). `pool.CursorName` (Task 1) is used by `internal/pool/pool.go` (Task 1) and `internal/api/operator_handlers.go` (Task 8) — one definition, two consumers.
