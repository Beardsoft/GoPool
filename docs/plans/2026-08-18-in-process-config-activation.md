# In-Process Config Activation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. REQUIRED: superpowers:test-driven-development for every task.

**Goal:** Activate setup and Settings writes without a user-run restart, on Compose and Swarm, by waiting/reloading in-process.

**Architecture:** Keep `CONFIG_FILE` as live source of truth and SQLite as revision/heartbeat store. Daemon waits for a missing file, retries readiness errors instead of exiting, and reloads when the file hash changes and no payout is in flight. API serves through a replaceable handler and swaps from setup-only to full routes after `POST /api/setup/complete`. UI polls until heartbeat hash matches.

**Tech Stack:** Go 1.25, `net/http` ServeMux, SQLite/sqlc, Vue 3 + Vitest, existing Compose/Swarm mounts.

**Design:** `docs/plans/2026-08-18-in-process-config-activation-design.md`

---

### Task 1: Daemon waits for a missing config file

**Files:**
- Create tests in: `internal/config/wait_test.go`
- Modify: `internal/config/config.go`
- Modify: `cmd/main.go`

**Step 1: Write the failing test**

```go
func TestWaitForDaemonReturnsOnceFileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = os.WriteFile(path, []byte(`{"payout_mode":"delegate","pool_fee_percentage":0.01,"min_payout_luna":1}`+"\n"), 0o600)
	}()

	cfg, err := WaitForDaemon(ctx, path, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForDaemon: %v", err)
	}
	if cfg.PayoutMode != "delegate" {
		t.Fatalf("payout_mode = %q", cfg.PayoutMode)
	}
}

func TestWaitForDaemonStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := WaitForDaemon(ctx, filepath.Join(t.TempDir(), "missing.json"), time.Millisecond)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
```

**Step 2: Verify RED**

Run: `go test ./internal/config -run TestWaitForDaemon -count=1`

Expected: FAIL because `WaitForDaemon` is undefined.

**Step 3: Minimal implementation**

Add `WaitForDaemon(ctx, path, retry)` that loops `LoadDaemon` until success or ctx done. Treat `os.ErrNotExist` as retry. Wire `cmd/main.go` to call it instead of fatal on missing file. Leave other LoadDaemon errors as fatal for this task (invalid JSON still fatal until Task 3).

**Step 4: Verify GREEN**

Run: `go test ./internal/config -run TestWaitForDaemon -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/config/wait_test.go internal/config/config.go cmd/main.go
git commit -m "Wait for config.json instead of crashing the daemon on first run."
```

---

### Task 2: Retry readiness instead of stopping the daemon

**Files:**
- Modify: `internal/pool/pool.go` (`Manager.Run`)
- Test: `internal/pool/pool_test.go` or a new `internal/pool/readiness_retry_test.go`

**Step 1: Write the failing test**

Cover a helper, e.g. `readinessRetryable(err) bool`, true for RPC/not-found/mismatch errors that today make `Run` return. Then a test that `ensureReady` records heartbeat and returns retryable rather than a hard stop—or that `Run` continues after a first `GetValidator` failure when the mock later succeeds.

If a full `Run` mock is heavy, test `ensureReady(ctx) error` in isolation: first call returns retryable wrapped error and writes heartbeat; second call nil.

**Step 2: Verify RED**

Run: `go test ./internal/pool -run 'TestReadinessRetry|TestEnsureReady' -count=1`

Expected: FAIL (missing helper or Run still returns).

**Step 3: Minimal implementation**

Extract the policy+validator+`validatePoolWallet` block from `Run` into `ensureReady`. On failure, keep `recordReadinessFailure`, then **loop with sleep** until success or `ctx.Done()`. Do not `return err` to `cmd/main` for those cases. `cmd/main.go` should not treat a transient readiness error as process death.

**Step 4: Verify GREEN**

Run: `go test ./internal/pool -count=1`

Expected: PASS, including existing `TestValidatePoolWallet`.

**Step 5: Commit**

```bash
git add internal/pool/pool.go internal/pool/*.go
git commit -m "Retry validator readiness instead of exiting the daemon."
```

---

### Task 3: Reload daemon when config hash changes and payouts are idle

**Files:**
- Modify: `internal/pool/pool.go` (tick loop)
- Modify: `internal/config/config.go` if hash helper lives there
- Test: `internal/pool/reload_test.go`

**Step 1: Write the failing tests**

```go
func TestShouldReloadWaitsForInFlightPayout(t *testing.T) {
	if shouldReload("aaa", "bbb", true) {
		t.Fatal("reload while payout in flight")
	}
	if !shouldReload("aaa", "bbb", false) {
		t.Fatal("reload when idle and hash changed")
	}
	if shouldReload("aaa", "aaa", false) {
		t.Fatal("reload when hash unchanged")
	}
}
```

Add a file-watch test: write a second config, `currentFileHash` changes.

**Step 2: Verify RED**

Run: `go test ./internal/pool -run TestShouldReload -count=1`

Expected: FAIL, `shouldReload` undefined.

**Step 3: Minimal implementation**

At the start of each successful tick (after `runPayouts` returns, so a tick’s payouts finished), if `CONFIG_FILE` hash != running hash, return a sentinel `errConfigChanged` from `Run`. `cmd/main.go` outer loop: wait/load, `NewManager`, `Run`; on `errConfigChanged`, loop (rebuild chain+manager). Define in-flight as: `runPayouts` still executing (so check **after** it returns) OR an awaiting-confirmation row if that query is cheap—prefer “reload only between ticks” which already waits for the payout function to return.

Invalid new file: log, keep running old manager, do not return `errConfigChanged`.

**Step 4: Verify GREEN**

Run: `go test ./internal/pool ./internal/config -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/pool/pool.go internal/pool/reload_test.go cmd/main.go internal/config/config.go
git commit -m "Reload the daemon between ticks when config.json hash changes."
```

---

### Task 4: API swaps from setup-only to full routes without exiting

**Files:**
- Modify: `internal/api/api.go`
- Modify: `internal/api/setup_handlers.go`
- Modify: `cmd/api/main.go` (server uses `a` as handler if `API` implements `http.Handler`)
- Test: `internal/api/setup_handlers_test.go`

**Step 1: Write the failing test**

Extend setup complete test: after 201, `GET /api/pool` on the **same** `a.Mux()` must not be `setup_required` / 404. Needs session secret file env or skip ValidateAPI pieces that block activate—`ActivateFromStore` should load file, set `a.cfg`, `a.setupOnly=false`, rebuild inner mux.

```go
func TestSetupCompleteActivatesPoolRoutes(t *testing.T) {
	// existing setup complete fixture...
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/pool", nil))
	if rec.Code == http.StatusNotFound || rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("pool still unavailable after setup: %d %s", rec.Code, rec.Body.String())
	}
}
```

Also assert `GET /` no longer 302s to `/setup`.

**Step 2: Verify RED**

Run: `go test ./internal/api -run TestSetupCompleteActivatesPoolRoutes -count=1`

Expected: FAIL, `/api/pool` still setup_required.

**Step 3: Minimal implementation**

- `API` holds `mu sync.Mutex` and `handler http.Handler`.
- `New` builds the initial mux into `handler`.
- `func (a *API) ServeHTTP(...)` delegates to `handler`.
- `Mux() http.Handler { return a }` so tests keep working.
- `ActivateConfigured(cfg, rpc)` sets fields, `setupOnly=false`, replaces `handler` with `a.buildMux()`.
- `handleSetupComplete` after successful `Save`: load config, `ValidateAPI` with session secret from env file, `chain.NewRPCOnly`, `ActivateConfigured`.
- `http.Server{Handler: a}` in `cmd/api/main.go`.

**Step 4: Verify GREEN**

Run: `go test ./internal/api -count=1`

Expected: PASS, including embed setup-only redirect tests (still setup-only **until** complete).

**Step 5: Commit**

```bash
git add internal/api cmd/api/main.go
git commit -m "Activate full API routes after setup without restarting the process."
```

---

### Task 5: Settings save updates live API config; pending means heartbeat lag

**Files:**
- Modify: `internal/api/operator_settings_handlers.go`
- Modify: `internal/configstore/store.go` (comment/`RestartRequired` meaning)
- Test: `internal/api/operator_settings_handlers_test.go`

**Step 1: Write the failing test**

After a successful PUT, `a.cfg.PoolFeePercentage` (or RPC URL) matches the saved settings. JSON still has `restart_required: true` while runtime hash differs—keep the field for compatibility but UI copy changes in Task 6.

**Step 2: Verify RED**

Run: `go test ./internal/api -run TestOperatorSettingsSave -count=1`

Expected: FAIL if cfg not updated.

**Step 3: Minimal implementation**

On save/restore success, `a.cfg` = loaded config (keep session secret), refresh RPC client if `rpc_url`/`network` changed. Do not os.Exit.

**Step 4: Verify GREEN**

Run: `go test ./internal/api -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/api/operator_settings_handlers.go internal/api/operator_settings_handlers_test.go
git commit -m "Apply settings saves to the running API process."
```

---

### Task 6: Wizard and Settings wait on heartbeat instead of showing compose restart

**Files:**
- Modify: `web/src/pages/setup/Setup.vue`
- Modify: `web/src/pages/setup/steps/ReviewLaunch.vue`
- Modify: `web/src/pages/operator/Settings.vue`
- Test: `web/src/pages/setup/Setup.test.ts`
- Test: existing Settings tests if any (`web/src/pages/operator/`)

**Step 1: Write the failing tests**

Setup launched state must not contain `docker compose`. After complete, poll `/api/operator/readiness` or `/api/pool` until hash/readiness; show `readiness_error` text when present.

Settings notice must not say “Restart required”; use “Activating…” while `restart_required` / hash mismatch.

**Step 2: Verify RED**

Run: `npx vitest run src/pages/setup/Setup.test.ts` from `web/`

Expected: FAIL (compose string still in template).

**Step 3: Minimal implementation**

- Setup success card: “Activating configuration…” + poll. On `/api/pool` 200, “Pool is live”. On readiness_error, show that error (e.g. validator not found).
- Poll `/api/pool` after mux swap; if still 401 on operator readiness, use pool or a small public `GET /api/setup/status` only while unconfigured—after activate, `/api/pool` is enough.
- Settings: rename notice; poll `GET /api/operator/settings` until `!restart_required`.

**Step 4: Verify GREEN**

Run: `npx vitest run src/pages/setup/Setup.test.ts src/pages/operator/` from `web/`

Expected: PASS.

**Step 5: Commit**

```bash
git add web/src/pages/setup web/src/pages/operator/Settings.vue web/src/pages/setup/Setup.test.ts
git commit -m "Poll daemon heartbeat after setup and settings instead of asking for a restart."
```

---

### Task 7: Docs

**Files:**
- Modify: `README.md`
- Modify: `deployments/SWARM.md`

**Step 1: Edit**

Remove “restart both services” as a required operator step after setup/settings. Describe wait/reload and that `readiness_error` can stay if the address is not a validator. Keep force-update only for image deploys.

**Step 2: Commit**

```bash
git add README.md deployments/SWARM.md
git commit -m "Document in-process config activation for Compose and Swarm."
```

---

### Task 8: Full verification

**Step 1:** `go test ./...`

Expected: PASS.

**Step 2:** `npx vitest run` in `web/`

Expected: PASS.

**Step 3:** Manual Compose or Swarm: start unconfigured, complete `/setup`, confirm no compose command, API serves `/api/pool` without `docker compose restart`, daemon replica stays 1/1 (may show readiness_error until a real validator exists).
