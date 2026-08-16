# Daemon Prometheus Metrics Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Expose a small Prometheus surface on the pool daemon so operators can alert on cursor lag, payouts, solvency, and validator state without scraping Nimiq node metrics.

**Architecture:** Private HTTP server on `metrics_addr` (default `:9100`). Package-level collectors in `internal/metrics`. Per-tick gauges from memory/SQL; counters next to existing log lines; live stake/balances on a 30s RPC throttle that piggybacks `GetValidator` when auto-reactivate already fetched it.

**Tech Stack:** Go, `prometheus/client_golang`, existing `internal/pool` + `nimiq-go` (`GetValidator`, `GetBalance`), sqlc/SQLite.

**Spec:** `docs/superpowers/specs/2026-08-13-prometheus-metrics-design.md`

---

### Task 1: Pure helpers (state, throttle, labels)

**Files:**
- Modify: `internal/pool/lifecycle.go` (add `validatorLiveState`)
- Modify: `internal/pool/lifecycle_test.go`
- Modify: `internal/pool/pool.go` (add `shouldRefreshGauges`, `chainGaugeInterval`)
- Modify: `internal/pool/pool_test.go`
- Modify: `internal/pool/payout.go` (add `payoutKindLabel`)
- Modify: `internal/pool/payout_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/pool/lifecycle_test.go` (add `"github.com/NimMiniApps/nimiq-go/rpc"` to imports):

```go
func TestValidatorLiveState(t *testing.T) {
	height := uint32(1000)
	future := uint32(2000)
	past := uint32(500)
	cases := []struct {
		name string
		v    rpc.Validator
		want string
	}{
		{"active", rpc.Validator{}, "active"},
		{"retired", rpc.Validator{Retired: true}, "retired"},
		{"inactive", rpc.Validator{InactivityFlag: &future}, "inactive"},
		{"inactive expired", rpc.Validator{InactivityFlag: &past}, "active"},
		{"jailed", rpc.Validator{JailedFrom: &future}, "jailed"},
		{"jail expired", rpc.Validator{JailedFrom: &past}, "active"},
	}
	for _, c := range cases {
		if got := validatorLiveState(&c.v, height); got != c.want {
			t.Errorf("%s: validatorLiveState = %q, want %q", c.name, got, c.want)
		}
	}
}
```

Append to `internal/pool/pool_test.go` (add `"time"` to imports):

```go
func TestShouldRefreshGauges(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	if !shouldRefreshGauges(time.Time{}, now) {
		t.Error("zero last must refresh")
	}
	if shouldRefreshGauges(now.Add(-29*time.Second), now) {
		t.Error("29s is too soon")
	}
	if !shouldRefreshGauges(now.Add(-30*time.Second), now) {
		t.Error("30s must refresh")
	}
}
```

Append to `internal/pool/payout_test.go`:

```go
func TestPayoutKindLabel(t *testing.T) {
	if got := payoutKindLabel(payoutDelegate); got != "delegate" {
		t.Errorf("delegate label = %q", got)
	}
	if got := payoutKindLabel(payoutTransfer); got != "transfer" {
		t.Errorf("transfer label = %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/pool/ -run 'TestValidatorLiveState|TestShouldRefreshGauges|TestPayoutKindLabel' -count=1`

Expected: FAIL — functions undefined

- [ ] **Step 3: Minimal implementation**

Add to `internal/pool/lifecycle.go` (needs `"github.com/NimMiniApps/nimiq-go/rpc"`):

```go
// validatorLiveState maps GetValidator fields to the 1/0 gauge enum.
// Inactive/jailed tests match handleElection.
func validatorLiveState(v *rpc.Validator, height uint32) string {
	switch {
	case v.Retired:
		return "retired"
	case v.InactivityFlag != nil && *v.InactivityFlag > height:
		return "inactive"
	case v.JailedFrom != nil && *v.JailedFrom > height:
		return "jailed"
	}
	return "active"
}
```

Add to `internal/pool/pool.go`:

```go
const chainGaugeInterval = 30 * time.Second

func shouldRefreshGauges(last, now time.Time) bool {
	return last.IsZero() || now.Sub(last) >= chainGaugeInterval
}
```

Add to `internal/pool/payout.go` next to the `payoutKind` consts:

```go
func payoutKindLabel(k payoutKind) string {
	if k == payoutDelegate {
		return "delegate"
	}
	return "transfer"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/pool/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/pool/lifecycle.go internal/pool/lifecycle_test.go internal/pool/pool.go internal/pool/pool_test.go internal/pool/payout.go internal/pool/payout_test.go
git commit -m "$(cat <<'EOF'
feat(pool): helpers for validator state, gauge throttle, and payout labels

EOF
)"
```

---

### Task 2: `internal/metrics` collectors and HTTP handler

**Files:**
- Create: `internal/metrics/metrics.go`
- Create: `internal/metrics/metrics_test.go`

- [ ] **Step 1: Add the module dependency**

Run: `go get github.com/prometheus/client_golang@latest`

- [ ] **Step 2: Write the failing test**

Create `internal/metrics/metrics_test.go`:

```go
package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerMetricsAndHealthz(t *testing.T) {
	ChainHead.Set(42)

	srv := httptest.NewServer(Handler())
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "gopool_chain_head") {
		t.Fatalf("metrics body missing gopool_chain_head:\n%s", text)
	}

	hz, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer hz.Body.Close()
	if hz.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz status %d", hz.StatusCode)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/metrics/ -count=1`

Expected: FAIL — package does not compile (`ChainHead`, `Handler` undefined)

- [ ] **Step 4: Minimal implementation**

Create `internal/metrics/metrics.go`:

```go
// Package metrics exposes GoPool daemon Prometheus collectors and the
// scrape HTTP handler. Collectors live on the default registry.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	ChainHead = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_chain_head",
		Help: "Current chain head height as seen by the daemon.",
	})
	LastProcessedHeight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_last_processed_height",
		Help: "DB cursor height last processed by the daemon.",
	})
	TickDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_tick_duration_seconds",
		Help: "Wall time of the last daemon tick.",
	})
	Stakers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_stakers",
		Help: "Staker count from the current in_progress epoch snapshot.",
	})
	DelegatedStake = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_delegated_stake_luna",
		Help: "Delegated stake (luna) from the current in_progress epoch snapshot.",
	})
	PayslipsPending = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_payslips_pending",
		Help: "Payslips waiting to be paid.",
	})
	PayslipsPendingLuna = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_payslips_pending_luna",
		Help: "Sum of pending payslip amounts (luna).",
	})
	PayslipsStuck = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_payslips_stuck",
		Help: "Payslips in out_for_payment or awaiting_confirmation.",
	})
	LiveStake = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_live_stake_luna",
		Help: "Validator.Balance from the last GetValidator (luna).",
	})
	LiveStakers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_live_stakers",
		Help: "Validator.NumStakers from the last GetValidator.",
	})
	ValidatorState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gopool_validator_state",
		Help: "Current validator state (1 for the active state, 0 otherwise).",
	}, []string{"state"})
	WalletBalance = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gopool_wallet_balance_luna",
		Help: "Liquid account balance (luna).",
	}, []string{"role"})
	RewardsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gopool_rewards_luna_total",
		Help: "Cumulative reward inherents collected (luna).",
	})
	PoolFeeTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gopool_pool_fee_luna_total",
		Help: "Cumulative pool fee withheld from rewards (luna).",
	})
	PayoutsSubmitted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gopool_payouts_submitted_total",
		Help: "Payout transactions submitted.",
	}, []string{"kind"})
	PayoutsConfirmed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gopool_payouts_confirmed_total",
		Help: "Payout transactions confirmed.",
	})
	PayoutsFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gopool_payouts_failed_total",
		Help: "Payout transactions that failed execution.",
	})
	RPCErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gopool_rpc_errors_total",
		Help: "RPC failures in the daemon loop.",
	}, []string{"op"})
)

func init() {
	prometheus.MustRegister(
		ChainHead, LastProcessedHeight, TickDuration,
		Stakers, DelegatedStake,
		PayslipsPending, PayslipsPendingLuna, PayslipsStuck,
		LiveStake, LiveStakers, ValidatorState, WalletBalance,
		RewardsTotal, PoolFeeTotal,
		PayoutsSubmitted, PayoutsConfirmed, PayoutsFailed,
		RPCErrors,
	)
}

// SetValidatorState sets the 1/0 enum gauges. Unknown state leaves all at 0.
func SetValidatorState(state string) {
	for _, s := range []string{"active", "inactive", "jailed", "retired"} {
		v := 0.0
		if s == state {
			v = 1
		}
		ValidatorState.WithLabelValues(s).Set(v)
	}
}

// Handler serves GET /metrics and GET /healthz.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/metrics/ -count=1`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/metrics/metrics.go internal/metrics/metrics_test.go
git commit -m "$(cat <<'EOF'
feat(metrics): Prometheus collectors and /metrics handler

EOF
)"
```

---

### Task 3: `metrics_addr` config

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Modify: `config.json`

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:

```go
package config

import "testing"

func TestValidateStillIgnoresMetricsAddr(t *testing.T) {
	cfg := Config{
		PayoutMode:        "delegate",
		PoolFeePercentage: 0.01,
		MetricsAddr:       "",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}
```

This will fail to compile until `MetricsAddr` exists.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/config/ -count=1`

Expected: FAIL — `MetricsAddr` undefined

- [ ] **Step 3: Minimal implementation**

Add to `Config` in `internal/config/config.go`:

```go
MetricsAddr string `mapstructure:"metrics_addr"`
```

In `LoadConfig`, after the existing `SetDefault` calls:

```go
viper.SetDefault("metrics_addr", ":9100")
```

Empty string remains valid (disables the listener). Do not add a Validate check.

Add to `config.json`:

```json
"metrics_addr": ":9100"
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/config/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go config.json
git commit -m "$(cat <<'EOF'
feat(config): metrics_addr for the daemon scrape listener

EOF
)"
```

---

### Task 4: Payslip stats and epoch snapshot queries

**Files:**
- Modify: `sql/queries.sql`
- Modify: `internal/db/queries.sql.go` (via `sqlc generate`, or hand-write to match sqlc style if `sqlc` is missing)

`sqlc` may not be installed (`which sqlc`). If missing, hand-write functions that `sqlc generate` would produce. If present, run `sqlc generate` from the repo root and do not hand-edit generated files.

- [ ] **Step 1: Add queries**

Append to `sql/queries.sql`:

```sql
-- name: GetPayslipStats :one
SELECT
    CAST(COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0) AS INTEGER) AS pending_count,
    CAST(COALESCE(SUM(CASE WHEN status = 'pending' THEN amount ELSE 0 END), 0) AS INTEGER) AS pending_luna,
    CAST(COALESCE(SUM(CASE WHEN status IN ('out_for_payment', 'awaiting_confirmation') THEN 1 ELSE 0 END), 0) AS INTEGER) AS stuck_count
FROM payslips;

-- name: GetCurrentEpochSnapshot :one
SELECT num_stakers, balance FROM epochs
WHERE status = 'in_progress'
ORDER BY number DESC
LIMIT 1;
```

- [ ] **Step 2: Generate or hand-write**

Run: `sqlc generate`

If `sqlc` is missing, append to `internal/db/queries.sql.go` (keep the file's existing style: exported `const` query string, row struct, method on `*Queries`):

```go
const GetPayslipStats = `-- name: GetPayslipStats :one
SELECT
    CAST(COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0) AS INTEGER) AS pending_count,
    CAST(COALESCE(SUM(CASE WHEN status = 'pending' THEN amount ELSE 0 END), 0) AS INTEGER) AS pending_luna,
    CAST(COALESCE(SUM(CASE WHEN status IN ('out_for_payment', 'awaiting_confirmation') THEN 1 ELSE 0 END), 0) AS INTEGER) AS stuck_count
FROM payslips
`

type GetPayslipStatsRow struct {
	PendingCount int64 `json:"pending_count"`
	PendingLuna  int64 `json:"pending_luna"`
	StuckCount   int64 `json:"stuck_count"`
}

func (q *Queries) GetPayslipStats(ctx context.Context) (GetPayslipStatsRow, error) {
	row := q.db.QueryRowContext(ctx, GetPayslipStats)
	var i GetPayslipStatsRow
	err := row.Scan(&i.PendingCount, &i.PendingLuna, &i.StuckCount)
	return i, err
}

const GetCurrentEpochSnapshot = `-- name: GetCurrentEpochSnapshot :one
SELECT num_stakers, balance FROM epochs
WHERE status = 'in_progress'
ORDER BY number DESC
LIMIT 1
`

type GetCurrentEpochSnapshotRow struct {
	NumStakers int64 `json:"num_stakers"`
	Balance    int64 `json:"balance"`
}

func (q *Queries) GetCurrentEpochSnapshot(ctx context.Context) (GetCurrentEpochSnapshotRow, error) {
	row := q.db.QueryRowContext(ctx, GetCurrentEpochSnapshot)
	var i GetCurrentEpochSnapshotRow
	err := row.Scan(&i.NumStakers, &i.Balance)
	return i, err
}
```

- [ ] **Step 3: Compile**

Run: `go test ./internal/db/ -count=1`

Expected: PASS (package may have no tests; compile is enough)

- [ ] **Step 4: Commit**

```bash
git add sql/queries.sql internal/db/queries.sql.go
git commit -m "$(cat <<'EOF'
feat(db): payslip stats and current epoch snapshot for gauges

EOF
)"
```

---

### Task 5: Observe gauges and counters from the daemon loop

**Files:**
- Modify: `internal/pool/pool.go`
- Modify: `internal/pool/election.go`
- Modify: `internal/pool/checkpoint.go`
- Modify: `internal/pool/payout.go`
- Modify: `internal/pool/confirmation.go`
- Modify: `internal/pool/lifecycle.go`

No new tests here: existing pool tests are pure helpers and do not construct a `Manager` with RPC. Counters/gauges are covered by Task 2's handler test plus the helpers in Task 1. After wiring, `go test ./internal/pool/ -count=1` must still pass.

- [ ] **Step 1: Add refresh state on Manager**

In `internal/pool/pool.go`, add to `Manager`:

```go
lastChainGaugeRefresh time.Time
lastValidatorObserve  time.Time
```

This is throttle state, not a metrics object.

- [ ] **Step 2: Add observe helpers on Manager**

Add to `internal/pool/pool.go` (import `github.com/Beardsoft/GoPool/internal/metrics` and `nimiq-go/rpc`):

```go
func (m *Manager) observeValidator(v *rpc.Validator, height uint32) {
	metrics.LiveStake.Set(float64(v.Balance))
	metrics.LiveStakers.Set(float64(v.NumStakers))
	metrics.SetValidatorState(validatorLiveState(v, height))
	m.lastValidatorObserve = time.Now()
}

func (m *Manager) observeTickGauges(ctx context.Context, head uint32, cursor int64, tickStart time.Time) {
	metrics.ChainHead.Set(float64(head))
	metrics.LastProcessedHeight.Set(float64(cursor))
	metrics.TickDuration.Set(time.Since(tickStart).Seconds())

	stats, err := m.queries.GetPayslipStats(ctx)
	if err == nil {
		metrics.PayslipsPending.Set(float64(stats.PendingCount))
		metrics.PayslipsPendingLuna.Set(float64(stats.PendingLuna))
		metrics.PayslipsStuck.Set(float64(stats.StuckCount))
	}
	snap, err := m.queries.GetCurrentEpochSnapshot(ctx)
	if err == nil {
		metrics.Stakers.Set(float64(snap.NumStakers))
		metrics.DelegatedStake.Set(float64(snap.Balance))
	}

	now := time.Now()
	if !shouldRefreshGauges(m.lastChainGaugeRefresh, now) {
		return
	}
	m.lastChainGaugeRefresh = now

	if shouldRefreshGauges(m.lastValidatorObserve, now) {
		v, err := m.chain.RPC.GetValidator(ctx, m.chain.Address())
		if err != nil {
			metrics.RPCErrors.WithLabelValues("validator").Inc()
		} else {
			m.observeValidator(v, head)
		}
	}

	bal, err := m.chain.RPC.GetBalance(ctx, m.chain.Address())
	if err != nil {
		metrics.RPCErrors.WithLabelValues("balance").Inc()
		return
	}
	metrics.WalletBalance.WithLabelValues("payout").Set(float64(bal))

	if m.cfg.PoolFeeWallet == "" {
		return
	}
	rewardAddr, err := nimiq.ParseAddress(m.cfg.PoolFeeWallet)
	if err != nil || rewardAddr == m.chain.Address() {
		return
	}
	rbal, err := m.chain.RPC.GetBalance(ctx, rewardAddr)
	if err != nil {
		metrics.RPCErrors.WithLabelValues("balance").Inc()
		return
	}
	metrics.WalletBalance.WithLabelValues("reward").Set(float64(rbal))
}
```

Need `nimiq "github.com/NimMiniApps/nimiq-go"` in `pool.go` imports for `ParseAddress`.

- [ ] **Step 3: Call observeTickGauges at the end of each tick**

In `Run`, capture `tickStart := time.Now()` after the ticker fires (start of work). After the payout/confirm/reactivate block, compute the cursor currently in the DB (the `cursor` variable from this tick, or `head` if the loop caught up) and call:

```go
processed := int64(head)
if start <= head {
	// if the height loop broke early, cursor is last successfully upserted;
	// re-read so lag is honest
	if c, err := m.queries.GetCursor(ctx, cursorName); err == nil {
		processed = c
	}
}
m.observeTickGauges(ctx, head, processed, tickStart)
```

Simpler and honest: always re-read the cursor after the height loop. If GetCursor fails (never started), pass `int64(m.policy.GenesisBlockNumber)` or `0`.

On `BlockNumber` error, before `continue`:

```go
metrics.RPCErrors.WithLabelValues("block_number").Inc()
```

On `processHeight` error, before `break`:

```go
metrics.RPCErrors.WithLabelValues("height").Inc()
```

Do **not** increment `height` on `UpsertCursor` SQL errors.

- [ ] **Step 4: Piggyback GetValidator**

In `handleElection`, after a successful `GetValidator` (the `validator, err :=` that does **not** take the `not_elected` branch), call `m.observeValidator(validator, height)`.

In `runAutoReactivate`, after a successful `GetValidator`, call `m.observeValidator(validator, head)` (use the `head` already fetched there; if `BlockNumber` has not run yet in that function, observe after head is loaded). If `GetValidator` fails for a real RPC reason it currently returns `nil` ("not a validator"); do not increment `reactivate` for that. On `BlockNumber` / send failure in that function, increment `reactivate`.

- [ ] **Step 5: Counters on money paths**

In `handleCheckpoint`, after a successful `InsertReward`, before returning:

```go
metrics.RewardsTotal.Add(float64(reward))
metrics.PoolFeeTotal.Add(float64(fee))
```

(`reward` is the inherent sum, `fee` is the pool cut from `splitReward`.)

In `runPayouts`, after a successful submit (the existing `"payout submitted"` log):

```go
metrics.PayoutsSubmitted.WithLabelValues(payoutKindLabel(kind)).Inc()
```

On payout `signAndSend` failure (existing error log), also:

```go
metrics.RPCErrors.WithLabelValues("payout").Inc()
```

In `runConfirmations`:

- `outcomeSucceeded` → `metrics.PayoutsConfirmed.Inc()`
- `outcomeFailed` → `metrics.PayoutsFailed.Inc()`
- `CheckTransaction` error that is **not** `rpc.ErrNotFound` → `metrics.RPCErrors.WithLabelValues("confirm").Inc()`

- [ ] **Step 6: Run tests**

Run: `go test ./internal/pool/ ./internal/metrics/ ./internal/config/ -count=1`

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/pool/pool.go internal/pool/election.go internal/pool/checkpoint.go internal/pool/payout.go internal/pool/confirmation.go internal/pool/lifecycle.go
git commit -m "$(cat <<'EOF'
feat(pool): record Prometheus gauges and payout/RPC counters

EOF
)"
```

---

### Task 6: Start the listener and expose the port

**Files:**
- Modify: `cmd/main.go`
- Modify: `internal/metrics/metrics.go` (add `Serve`)
- Modify: `deployments/docker-compose.yml`

- [ ] **Step 1: Add `Serve`**

Add to `internal/metrics/metrics.go` (import `context` and `errors`):

```go
// Serve listens on addr until ctx is cancelled. Empty addr is a no-op.
func Serve(ctx context.Context, addr string) error {
	if addr == "" {
		return nil
	}
	srv := &http.Server{Addr: addr, Handler: Handler()}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
```

- [ ] **Step 2: Start it from main**

In `cmd/main.go`, after `signal.NotifyContext` and **before** the validator-CLI early return (CLI must not start the scrape server), if running the daemon:

```go
if cfg.MetricsAddr != "" {
	go func() {
		if err := metrics.Serve(ctx, cfg.MetricsAddr); err != nil {
			logger.Logger.Error("metrics server", zap.Error(err))
		}
	}()
}
```

Import `github.com/Beardsoft/GoPool/internal/metrics`.

Place this immediately before `manager.Run(ctx)`, not before the CLI branch.

- [ ] **Step 3: Compose port**

In `deployments/docker-compose.yml`, add `9100:9100` next to `8080:8080`.

- [ ] **Step 4: Compile**

Run: `go test ./... -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/main.go internal/metrics/metrics.go deployments/docker-compose.yml
git commit -m "$(cat <<'EOF'
feat(daemon): listen on metrics_addr for Prometheus scrapes

EOF
)"
```

---

### Task 7: Smoke-check the scrape payload

- [ ] **Step 1: Confirm metric names**

Run: `go test ./internal/metrics/ -count=1 -v`

Expected: PASS, `TestHandlerMetricsAndHealthz` still finds `gopool_chain_head`

- [ ] **Step 2: Manual check (optional, if a node is up)**

Run the daemon, then:

```
curl -sS localhost:9100/healthz
curl -sS localhost:9100/metrics | grep '^gopool_'
```

Expect `ok` and the `gopool_*` names from the spec. Live/wallet series may be 0 until the first 30s refresh.

Do not provision Grafana dashboards or alerts in this change.
