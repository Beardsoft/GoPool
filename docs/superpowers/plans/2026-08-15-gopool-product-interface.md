# GoPool Product Interface Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Nimiq-native public staking-pool experience, first-run setup assistant, and operator console that makes GoPool straightforward for another person to launch and operate safely.

**Architecture:** Keep the daemon as the only signing process and the API as a signer-free read/control plane over shared SQLite. Add forward-only migrations, a focused operational recorder, an atomic non-secret configuration store, and setup-only API mode; build the Vue application from shared Nimiq primitives into separate public, staker, and operator surfaces.

**Tech Stack:** Go 1.25, SQLite, sqlc, `net/http`, zap, Prometheus, Vue 3, Vue Router 4, TypeScript 5.7, Vite 6, Vitest, Vue Test Utils, Chart.js 4, Nimiq Hub API, local Nimiq UI Kit.

**Spec:** `docs/superpowers/specs/2026-08-15-gopool-product-interface-design.md`

## Global Constraints

- Treat all existing uncommitted daemon, API, frontend, deployment, metrics, alert, SSE, chart, and onboarding work as user-owned baseline; never discard it wholesale.
- Keep the daemon as the only process that reads `POOL_PRIVATE_KEY_FILE`, derives the validator address, or signs transactions.
- The API container must not mount or read the validator key.
- Store NIM values as integer luna and render them using `1 NIM = 100000 luna` with at most five decimal places.
- Use Muli for interface text and Fira Mono for addresses, hashes, exact timestamps, and dense numeric telemetry.
- Use the Nimiq UI Kit tokens, bottom-right radial gradients, 8px spacing rhythm, 10px cards, opacity ladder, and official semantic colors.
- Follow device theme on first visit, persist a manual override, and render each page wholly light or wholly dark.
- Do not add `@nimiq/vue-components`; implement the required token-driven patterns in the existing Vue application.
- Every operator mutation requires server-side validation, explicit review/confirmation, an audit record, and reconciliation with daemon state.
- Do not use optimistic UI for configuration writes or on-chain actions.
- Keep automatic payouts as the default; make queue state and failures visible without requiring manual approval for every payout.
- Every phase must end with a buildable application and a focused commit.

## File and Responsibility Map

### Frontend foundation

- `web/src/styles/nimiq-tokens.css` — vendored Nimiq tokens.
- `web/src/style.css` — theme mappings, reset, layout, focus, responsive table, skeleton, notice, and motion rules.
- `web/src/types/api.ts` — shared wire types.
- `web/src/utils/format.ts` — luna/NIM, address, timestamp, and status formatting.
- `web/src/composables/useTheme.ts`, `useLiveStatus.ts` — theme and SSE state.
- `web/src/components/ui/*` — amount, address, status, icon, skeleton, notice, empty state, chart/table, and hold-confirm primitives.
- `web/src/layouts/*` — public, operator, and setup shells.
- `web/src/pages/*` — public/staker pages; `web/src/pages/operator/*` and `web/src/pages/setup/*` — role-specific pages.

### Backend and persistence

- `internal/db/migrate.go`, `internal/db/migrations/*.sql` — ordered migrations.
- `schema/scheme.sql`, `sql/queries.sql`, generated `internal/db/*` — final schema and typed queries.
- `internal/ops/*` — heartbeat, snapshots, activity, retention, and redaction.
- `internal/api/operator_*_handlers.go` — overview, operations, activity, alerts, and settings endpoints.
- `internal/configstore/*` — atomic config write, redaction, revisions, concurrency, and restore.
- `internal/api/setup_*.go` — bootstrap session and setup-only routes.

### Delivery

- `deployments/*`, `devlab/docker-compose.pool.yml` — distinct API/daemon config modes and daemon-only key secret.
- `README.md`, `deployments/SWARM.md` — first-run, restart, recovery, and secret instructions.

### Shared test helper contracts

- `internal/db/migrate_test.go` defines `openMemoryDB(t) *sql.DB`, `applyLegacySchema(t, db)`, and `assertTableExists(t, db, name)`. `openMemoryDB` opens `:memory:`, registers cleanup, and enables foreign keys; `applyLegacySchema` executes the pre-migration schema string; `assertTableExists` queries `sqlite_master` and fails when count is not one.
- `internal/ops/recorder_test.go` defines `testQueries(t) *db.Queries` by opening an in-memory DB and calling `db.Migrate`; `listEvents(t, q)` calls the unfiltered event query with cursor `math.MaxInt64` and limit 100.
- Create `internal/api/operator_test_helpers_test.go` in Task 8. It defines `seedRuntimeStatus`, `seedFailedPayout`, `seedAction`, `assertPostStatus`, `postJSON`, `putJSON`, `setupTestAPI`, and `configuredOperatorAPI`. Each uses generated queries and `httptest`; JSON helpers marshal their body, set `Content-Type`, optionally attach the supplied cookie, and return the recorder.
- Create `web/src/test/helpers.ts` in Task 1 with `mockFetch(path, body, status = 200)`, which installs a route-keyed `fetch` mock. Task 4 adds complete `positionFixture` and `historyFixture` constants after `types/api.ts` exists. Task 9 adds `overviewFixture(overrides)` and `operatorTestGlobals` with a memory router. Task 12 adds `mockAlertResponse`. Task 15 adds `setupTestGlobals`, `mockSetupValidation`, `goToEconomics`, and `loadSettings`; `loadSettings` drives the mounted page until its initial request resolves.

---

## Phase 1 — Foundation and Public Experience

### Task 1: Restore the frontend build and install the component-test harness

**Files:**
- Modify: `web/src/App.vue`
- Modify: `web/package.json`
- Modify: `web/package-lock.json`
- Create: `web/vitest.config.ts`
- Create: `web/src/utils/format.ts`
- Create: `web/src/utils/format.test.ts`
- Create: `web/src/test/helpers.ts`

**Interfaces:**
- Produces: `formatNim(luna: number): string`, `formatPercent(value: number): string`, `shortAddress(address: string): string`.
- Produces: `npm test -- --run` as the frontend unit-test gate.

- [ ] **Step 1: Write failing formatting tests**

```ts
import { expect, it } from 'vitest'
import { formatNim, formatPercent, shortAddress } from './format'

it('formats Nimiq values from luna', () => {
  expect(formatNim(1_842_193_612_345)).toBe('18,421,936.12345')
  expect(formatNim(100_000)).toBe('1')
  expect(formatPercent(0.025)).toBe('2.50%')
  expect(shortAddress('NQ12 8D4K AAAA BBBB CCCC DDDD EEEE FFFF GGGG')).toBe('NQ12 8D4K…FFFF GGGG')
})
```

- [ ] **Step 2: Add Vitest and verify the test fails for the missing module**

Run:

```bash
cd web
npm install --save-dev vitest@^3.2.4 @vue/test-utils@^2.4.6 happy-dom@^18.0.1
npm test -- --run src/utils/format.test.ts
```

Expected: FAIL because `format.ts` does not exist.

- [ ] **Step 3: Add test scripts/config, formatting functions, and repair the import**

```json
"scripts": {
  "dev": "vite",
  "build": "vue-tsc --noEmit && vite build",
  "test": "vitest"
}
```

```ts
// web/src/utils/format.ts
export function formatNim(luna: number): string {
  return (luna / 100_000).toLocaleString('en-US', { maximumFractionDigits: 5 })
}
export function formatPercent(fraction: number): string {
  return `${(fraction * 100).toFixed(2)}%`
}
export function shortAddress(address: string): string {
  const groups = address.trim().split(/\s+/)
  return groups.length >= 4 ? `${groups.slice(0, 2).join(' ')}…${groups.slice(-2).join(' ')}` : address
}
```

Configure Vitest with Vue and `happy-dom`. Change `App.vue` to import `ref` and `onMounted` from `vue`, not `vue-router`.

Create the shared fetch helper with an explicit route map:

```ts
// web/src/test/helpers.ts
import { vi } from 'vitest'
const responses = new Map<string, { body: unknown; status: number }>()
export function mockFetch(path: string, body: unknown, status = 200) {
  responses.set(path, { body, status })
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString()
    const hit = responses.get(url)
    if (!hit) return new Response(JSON.stringify({ code: 'not_mocked', error: url }), { status: 500 })
    return new Response(JSON.stringify(hit.body), { status: hit.status, headers: { 'Content-Type': 'application/json' } })
  }))
}
```

- [ ] **Step 4: Run the focused and production gates**

Run: `cd web && npm test -- --run src/utils/format.test.ts && npm run build`

Expected: tests PASS and the current production build succeeds.

- [ ] **Step 5: Commit**

```bash
git add web/package.json web/package-lock.json web/vitest.config.ts web/src/App.vue web/src/utils/format.ts web/src/utils/format.test.ts web/src/test/helpers.ts
git commit -m "test(web): restore build and add unit harness"
```

### Task 2: Add the Nimiq token foundation and global theme state

**Files:**
- Create: `web/src/styles/nimiq-tokens.css`
- Modify: `web/src/style.css`
- Modify: `web/src/main.ts`
- Create: `web/src/composables/useTheme.ts`
- Create: `web/src/composables/useTheme.test.ts`

**Interfaces:**
- Produces: `type ThemePreference = 'system' | 'light' | 'dark'`.
- Produces: `useTheme(): { theme, preference, setPreference, toggleTheme }`.

- [ ] **Step 1: Write the failing theme test**

```ts
it('uses system preference until manually overridden', () => {
  vi.stubGlobal('matchMedia', () => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
  const state = useTheme()
  expect(state.theme.value).toBe('dark')
  state.setPreference('light')
  expect(document.documentElement.dataset.theme).toBe('light')
  expect(localStorage.getItem('gopool-theme')).toBe('light')
})
```

- [ ] **Step 2: Run it and verify the composable is missing**

Run: `cd web && npm test -- --run src/composables/useTheme.test.ts`

Expected: FAIL resolving `useTheme`.

- [ ] **Step 3: Vendor tokens and implement one controller**

Run:

```bash
cp /home/maestro/Documents/projects/nimiq-ui-kit/tokens/tokens.css web/src/styles/nimiq-tokens.css
```

Import tokens before `style.css`. Use one module-level preference ref, a computed effective theme, a `prefers-color-scheme` listener, and `document.documentElement.dataset.theme`. Remove duplicate/broken custom properties from the current stylesheet. Map complete themes:

```css
:root { --app-bg: var(--nimiq-light-gray); --surface-1: white; --app-text: var(--text-100); --app-muted: var(--text-60); --app-border: var(--text-10); }
:root[data-theme='dark'] { --app-bg: #151833; --surface-1: #1f2348; --app-text: rgba(255,255,255,.96); --app-muted: rgba(255,255,255,.6); --app-border: rgba(255,255,255,.1); }
```

- [ ] **Step 4: Verify theme and build gates**

Run: `cd web && npm test -- --run src/composables/useTheme.test.ts && npm run build`

Expected: PASS in both theme branches.

- [ ] **Step 5: Commit**

```bash
git add web/src/styles/nimiq-tokens.css web/src/style.css web/src/main.ts web/src/composables/useTheme.ts web/src/composables/useTheme.test.ts
git commit -m "feat(web): add Nimiq light and dark themes"
```

### Task 3: Build UI primitives and role-aware layouts

**Files:**
- Create: `web/src/components/ui/AppIcon.vue`
- Create: `web/src/components/ui/NimAmount.vue`
- Create: `web/src/components/ui/AddressIdentity.vue`
- Create: `web/src/components/ui/StatusBadge.vue`
- Create: `web/src/components/ui/AppNotice.vue`
- Create: `web/src/components/ui/EmptyState.vue`
- Create: `web/src/components/ui/SkeletonBlock.vue`
- Create: `web/src/components/ui/HoldConfirmButton.vue`
- Create: `web/src/components/ui/ui.test.ts`
- Create: `web/src/components/AppHeader.vue`
- Create: `web/src/components/OperatorNav.vue`
- Create: `web/src/layouts/PublicLayout.vue`
- Create: `web/src/layouts/OperatorLayout.vue`
- Create: `web/src/layouts/SetupLayout.vue`
- Modify: `web/src/App.vue`
- Modify: `web/src/router.ts`

**Interfaces:**
- `NimAmount` props: `{ luna: number; showSymbol?: boolean }`.
- `AddressIdentity` props: `{ address: string; compact?: boolean; copyable?: boolean }`.
- `HoldConfirmButton` emits `confirm` after a 2,000ms pointer hold or explicit keyboard confirmation.

- [ ] **Step 1: Write failing amount and hold tests**

```ts
it('renders luna as NIM', () => expect(mount(NimAmount, { props: { luna: 123_456_789 } }).text()).toContain('1,234.56789 NIM'))
it('does not confirm before 2 seconds', async () => {
  vi.useFakeTimers()
  const wrapper = mount(HoldConfirmButton)
  await wrapper.trigger('pointerdown')
  vi.advanceTimersByTime(1_999)
  expect(wrapper.emitted('confirm')).toBeFalsy()
  vi.advanceTimersByTime(1)
  expect(wrapper.emitted('confirm')).toHaveLength(1)
})
```

- [ ] **Step 2: Run tests and verify missing modules fail**

Run: `cd web && npm test -- --run src/components/ui/ui.test.ts`

Expected: FAIL resolving components.

- [ ] **Step 3: Implement primitives and nested layouts**

Use the Nimiq icon sprite through inline SVG `<use>`; remove emoji production icons. Give hold confirmation pointer cancellation, progress text, `aria-describedby`, and keyboard confirmation. Make `App.vue` only render `<RouterView />`. Nest public routes under `PublicLayout` and `/operator/*` under `OperatorLayout`; setup uses `SetupLayout`.

- [ ] **Step 4: Run tests and build**

Run: `cd web && npm test -- --run src/components/ui/ui.test.ts && npm run build`

Expected: PASS; `/` and `/operator` chunks compile.

- [ ] **Step 5: Commit**

```bash
git add web/src/components web/src/layouts web/src/App.vue web/src/router.ts
git commit -m "feat(web): add role-aware Nimiq application shell"
```

### Task 4: Redesign public overview and performance

**Files:**
- Create: `web/src/types/api.ts`
- Rename and redesign: `web/src/pages/RewardsChart.vue` → `web/src/pages/Performance.vue`
- Create: `web/src/components/RewardChart.vue`
- Create: `web/src/components/RewardChart.test.ts`
- Modify: `web/src/pages/PoolOverview.vue`
- Modify: `web/src/pages/Epochs.vue`
- Modify: `web/src/pages/EpochDetail.vue`
- Modify: `web/src/router.ts`
- Modify: `web/src/test/helpers.ts`

**Interfaces:**
- Produces `PoolStatus`, `EpochSummary`, `EpochDetail`, and `RewardPoint` in `types/api.ts`.
- `RewardChart` props: `{ points: RewardPoint[]; range: '20e' | '90d' | 'all' }`.
- Consumes existing pool, policy, reward, and epoch APIs.

- [ ] **Step 1: Write a failing chart accessibility test**

```ts
it('summarizes plotted rewards', () => {
  const wrapper = mount(RewardChart, { props: { points: [{ epoch_number: 8, total_amount: 250_000, total_fee: 5_000, batches: 2 }], range: '20e' } })
  expect(wrapper.get('canvas').attributes('aria-label')).toContain('Epoch 8')
  expect(wrapper.text()).toContain('2.5 NIM')
})
```

- [ ] **Step 2: Run and verify the missing chart failure**

Run: `cd web && npm test -- --run src/components/RewardChart.test.ts`

Expected: FAIL resolving `RewardChart.vue`.

- [ ] **Step 3: Implement homepage and performance composition**

Homepage order: concise trust promise, live health/epoch, delegated stake, rewards, stakers, fee, address lookup, reward explanation, activity. Rename the current `RewardsChart.vue` work to `Performance.vue`; preserve both Chart.js datasets (per-epoch and cumulative), then add range controls, accessible data tables, epoch list, and epoch drill-down. Convert every luna field with `NimAmount` and add typed staker/history fixtures to `web/src/test/helpers.ts`.

- [ ] **Step 4: Run tests and build**

Run: `cd web && npm test -- --run src/components/RewardChart.test.ts && npm run build`

Expected: PASS with no raw luna on the redesigned pages.

- [ ] **Step 5: Commit**

```bash
git add web/src/types/api.ts web/src/pages/PoolOverview.vue web/src/pages/Performance.vue web/src/pages/Epochs.vue web/src/pages/EpochDetail.vue web/src/components/RewardChart.vue web/src/components/RewardChart.test.ts web/src/router.ts web/src/test/helpers.ts
git commit -m "feat(web): redesign public pool performance"
```

### Task 5: Consolidate staker lookup and My stake

**Files:**
- Create: `web/src/components/StakerPosition.vue`
- Create: `web/src/components/StakerPosition.test.ts`
- Modify: `web/src/pages/StakerLookup.vue`
- Modify: `web/src/pages/MyDashboard.vue`
- Delete after content migration: `web/src/pages/Onboard.vue`
- Modify: `web/src/router.ts`

**Interfaces:**
- `StakerPosition` props: `{ position: StakerDetail; history: StakerHistory; exportUrl: string }`.
- Public lookup resolves a route address; My stake resolves `/api/me`; both render the same component.

- [ ] **Step 1: Write the failing shared-position test**

```ts
it('shows stake, rewards and export', () => {
  const wrapper = mount(StakerPosition, { props: { position: positionFixture, history: historyFixture, exportUrl: '/api/me/payslips.csv' } })
  expect(wrapper.text()).toContain('5 NIM')
  expect(wrapper.text()).toContain('2.5 NIM')
  expect(wrapper.get('a[download]').attributes('href')).toContain('payslips.csv')
})
```

- [ ] **Step 2: Run and verify the component is missing**

Run: `cd web && npm test -- --run src/components/StakerPosition.test.ts`

Expected: FAIL resolving the component.

- [ ] **Step 3: Implement one canonical position view**

Move unique onboarding copy into `StakerLookup.vue`. Make `MyDashboard.vue` render Hub sign-in and the same `StakerPosition`. Redirect `/onboard` to `/stakers`, then remove `Onboard.vue`. Encode addresses in routes and CSV links.

- [ ] **Step 4: Run tests and build**

Run: `cd web && npm test -- --run src/components/StakerPosition.test.ts && npm run build`

Expected: PASS; `/onboard` redirects.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/StakerPosition* web/src/pages/StakerLookup.vue web/src/pages/MyDashboard.vue web/src/pages/Onboard.vue web/src/router.ts
git commit -m "feat(web): unify staker position flows"
```

## Phase 2 — Operator Observability

### Task 6: Add forward-only migrations and operator-console tables

**Files:**
- Create: `internal/db/migrate.go`
- Create: `internal/db/migrate_test.go`
- Create: `internal/db/migrations/001_initial.sql`
- Create: `internal/db/migrations/002_operator_console.sql`
- Modify: `internal/db/init.go`
- Modify: `schema/scheme.sql`
- Modify: `sql/queries.sql`
- Regenerate: `internal/db/models.go`
- Regenerate: `internal/db/queries.sql.go`

**Interfaces:**
- Produces `Migrate(db *sql.DB) error` and `schema_migrations(version INTEGER PRIMARY KEY, applied_at TIMESTAMP)`.
- Produces typed queries for runtime status, health snapshots, events, alert deliveries, config revisions, enriched actions, and payouts.

- [ ] **Step 1: Write failing fresh and legacy migration tests**

```go
func TestMigrateFreshAndLegacyDatabases(t *testing.T) {
    for _, tc := range []struct{name string; legacy bool}{{"fresh", false}, {"legacy", true}} {
        t.Run(tc.name, func(t *testing.T) {
            db := openMemoryDB(t)
            if tc.legacy { applyLegacySchema(t, db) }
            if err := Migrate(db); err != nil { t.Fatal(err) }
            for _, table := range []string{"runtime_status", "health_snapshots", "operator_events", "alert_deliveries", "config_revisions"} {
                assertTableExists(t, db, table)
            }
        })
    }
}
```

- [ ] **Step 2: Run and verify `Migrate` is missing**

Run: `go test ./internal/db -run TestMigrateFreshAndLegacyDatabases -v`

Expected: FAIL because `Migrate` does not exist.

- [ ] **Step 3: Implement the migration ledger and schema**

Embed ordered files with `//go:embed migrations/*.sql`. Execute each unapplied migration inside a transaction and record its version only after success. `001_initial.sql` reproduces the legacy schema. `002_operator_console.sql` creates the five new tables and adds `requested_by`, `requested_at`, `updated_at`, `state`, `error_summary`, and `correlation_id` to `validator_actions`. Make `schema/scheme.sql` the final schema snapshot used by sqlc and API tests. Replace `executeSchema` in `InitDB` with `Migrate`.

- [ ] **Step 4: Add exact queries, regenerate, and test**

Include queries such as:

```sql
-- name: UpsertRuntimeStatus :exec
INSERT INTO runtime_status (id, heartbeat_at, daemon_version, config_hash, derived_validator_address, validator_state, last_processed_height, chain_head, last_tick_ms, rpc_ok, readiness_error)
VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET heartbeat_at=excluded.heartbeat_at, daemon_version=excluded.daemon_version, config_hash=excluded.config_hash, derived_validator_address=excluded.derived_validator_address, validator_state=excluded.validator_state, last_processed_height=excluded.last_processed_height, chain_head=excluded.chain_head, last_tick_ms=excluded.last_tick_ms, rpc_ok=excluded.rpc_ok, readiness_error=excluded.readiness_error;

-- name: InsertOperatorEvent :one
INSERT INTO operator_events (severity, category, source, event_type, summary, context_json, actor_address, correlation_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id;
```

Run: `sqlc generate && go test ./internal/db -v`

Expected: generated models contain all new tables and tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db schema/scheme.sql sql/queries.sql
git commit -m "feat(db): add operator console persistence"
```

### Task 7: Record heartbeat, telemetry, and structured activity

**Files:**
- Create: `internal/ops/types.go`
- Create: `internal/ops/recorder.go`
- Create: `internal/ops/recorder_test.go`
- Modify: `internal/pool/pool.go`
- Modify: `internal/pool/checkpoint.go`
- Modify: `internal/pool/election.go`
- Modify: `internal/pool/payout.go`
- Modify: `internal/pool/confirmation.go`
- Modify: `internal/pool/lifecycle.go`
- Modify: `internal/chain/client.go`
- Modify: `cmd/main.go`

**Interfaces:**
- Produces `ops.Recorder.RecordEvent`, `RecordHeartbeat`, `RecordSnapshot`, and `Prune`.
- Produces `ops.EventInput`, `Heartbeat`, and `Snapshot` with explicit fields.
- `pool.NewManager` gains `pool.WithRecorder(*ops.Recorder)` and defaults to no-op recording.

- [ ] **Step 1: Write a failing redaction test**

```go
func TestRecorderRedactsSecretContext(t *testing.T) {
    q := testQueries(t)
    r := NewRecorder(q)
    err := r.RecordEvent(context.Background(), EventInput{
        Severity: "warning", Category: "payout", Source: "daemon", Type: "payout_failed",
        Summary: "Payout submission failed", Context: map[string]any{"address": "NQ…", "private_key": "must-not-store"},
    })
    if err != nil { t.Fatal(err) }
    got := listEvents(t, q)
    if strings.Contains(got[0].ContextJson, "must-not-store") { t.Fatal("secret persisted") }
}
```

- [ ] **Step 2: Run and verify the package is missing**

Run: `go test ./internal/ops -v`

Expected: FAIL because `internal/ops` does not exist.

- [ ] **Step 3: Implement recorder and pool transition hooks**

Recursively replace context keys matching `private_key`, `session_secret`, `token`, `authorization`, or `cookie` with `[redacted]`. Record snapshots at most once per minute. Prune snapshots older than 30 days and non-audit events older than 90 days. Write heartbeat/snapshot after each tick; record checkpoint, election, payout submitted/confirmed/failed, validator action, RPC failure, and reactivation events. Use one correlation ID per payout or validator-action lifecycle. Before normal operation, compare `chain.Address()` to `cfg.ValidatorAddress`; write a readiness error and return without processing if they differ. `Heartbeat.ConfigHash` may remain empty until Task 13 supplies the canonical editable-config hash.

- [ ] **Step 4: Run focused and regression tests**

Run:

```bash
go test ./internal/ops -v
go test ./internal/pool -v
```

Expected: PASS with payout and lifecycle behavior unchanged.

- [ ] **Step 5: Commit**

```bash
git add internal/ops internal/pool/pool.go internal/pool/checkpoint.go internal/pool/election.go internal/pool/payout.go internal/pool/confirmation.go internal/pool/lifecycle.go internal/chain/client.go cmd/main.go
git commit -m "feat(ops): persist daemon health and activity"
```

### Task 8: Add overview, readiness, telemetry, and activity APIs

**Files:**
- Create: `internal/api/operator_overview_handlers.go`
- Create: `internal/api/operator_overview_handlers_test.go`
- Modify: `internal/api/api.go`
- Modify: `internal/api/operator_handlers.go`
- Modify: `internal/api/testdb.go`
- Create: `internal/api/operator_test_helpers_test.go`

**Interfaces:**
- Produces `GET /api/operator/overview`, `GET /api/operator/readiness`, `GET /api/operator/telemetry?metric=&from=&to=&bucket=`, `GET /api/operator/activity?severity=&category=&from=&to=&cursor=&limit=`, and `GET /api/operator/activity/export?...`.
- All use `requireOperator` and stable `{ "code": string, "error": string }` errors.

- [ ] **Step 1: Write the failing overview test**

```go
func TestOperatorOverviewReturnsHealthAndAttention(t *testing.T) {
    a, cookie, _ := operatorTestAPI(t)
    seedRuntimeStatus(t, a.queries, "active", 120, 122, true)
    seedFailedPayout(t, a.queries)
    rec := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/api/operator/overview", nil)
    req.AddCookie(cookie)
    a.Mux().ServeHTTP(rec, req)
    if rec.Code != http.StatusOK { t.Fatalf("%d: %s", rec.Code, rec.Body.String()) }
    var got operatorOverviewResponse
    json.NewDecoder(rec.Body).Decode(&got)
    if got.Status != "attention" || got.ChainLag != 2 || len(got.Attention) != 1 { t.Fatalf("%+v", got) }
}
```

- [ ] **Step 2: Run and verify the route returns 404**

Run: `go test ./internal/api -run 'TestOperator(Overview|Readiness|Telemetry|Activity)' -v`

Expected: FAIL with 404 or missing response types.

- [ ] **Step 3: Implement bounded response assembly**

Overview returns readiness, lag, payout/validator summary, attention, and 20 events. Compute `wallet_runway_days` as payout-wallet balance divided by the greater of one luna or the average confirmed daily payout over the previous 30 complete UTC days; return `null` when there are no confirmed payouts. Telemetry rejects unknown metrics and ranges over 31 days, returns no more than 500 ordered points, and accepts explicit buckets. Activity caps `limit` at 100 and uses an integer cursor. Activity export uses the same filters, caps exports at 10,000 rows, and returns CSV unless `format=json`. Introduce:

```go
func writeAPIError(w http.ResponseWriter, status int, code, message string) {
    writeJSON(w, status, map[string]string{"code": code, "error": message})
}
```

Keep `writeError` as a compatibility wrapper while migrating old handlers.

- [ ] **Step 4: Run API and full Go tests**

Run: `go test ./internal/api -v && go test ./...`

Expected: PASS; non-operators receive 403.

- [ ] **Step 5: Commit**

```bash
git add internal/api/operator_overview_handlers.go internal/api/operator_overview_handlers_test.go internal/api/operator_test_helpers_test.go internal/api/api.go internal/api/operator_handlers.go internal/api/testdb.go
git commit -m "feat(api): expose operator health and activity"
```

### Task 9: Build operator Overview and Activity pages

**Files:**
- Create: `web/src/pages/operator/Overview.vue`
- Create: `web/src/pages/operator/Activity.vue`
- Create: `web/src/pages/operator/Overview.test.ts`
- Create: `web/src/components/TelemetryChart.vue`
- Create: `web/src/composables/useLiveStatus.ts`
- Create: `web/src/composables/useLiveStatus.test.ts`
- Modify: `web/src/api.ts`
- Modify: `web/src/types/api.ts`
- Modify: `web/src/router.ts`
- Modify: `web/src/components/OperatorNav.vue`
- Modify: `web/src/test/helpers.ts`
- Modify: `web/src/pages/PoolOverview.vue`
- Delete: `web/src/composables/useEvents.ts`

**Interfaces:**
- `apiGet` throws `ApiError { status: number; code: string; message: string }`.
- `useLiveStatus` returns `{ state: 'connecting'|'live'|'paused'; lastEventAt; reconnect() }`.

- [ ] **Step 1: Write failing hierarchy and SSE tests**

```ts
it('renders attention before telemetry', async () => {
  mockFetch('/api/operator/overview', overviewFixture({ status: 'attention' }))
  const wrapper = mount(Overview)
  await flushPromises()
  const attention = wrapper.get('[data-section="attention"]').element
  const telemetry = wrapper.get('[data-section="telemetry"]').element
  expect(attention.compareDocumentPosition(telemetry) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})
```

- [ ] **Step 2: Run and verify missing modules fail**

Run: `cd web && npm test -- --run src/pages/operator/Overview.test.ts src/composables/useLiveStatus.test.ts`

Expected: FAIL resolving new modules.

- [ ] **Step 3: Implement daily hierarchy and stale/live behavior**

Order the page: global status, four metrics, attention, validator, telemetry, activity. Do not add an opposite-theme footer. Activity adds filters, cursor pagination, expandable redacted context, and export URL. `useLiveStatus` owns one `EventSource`, changes to paused on error, reconnects with bounded backoff, triggers authoritative refetch, and leaves current data visible with a stale notice. Move the public overview to this composable and delete the current DOM-building `useEvents.ts`; render toast/status content through Vue templates so event fields are text, not `innerHTML`. Add `overviewFixture` and `operatorTestGlobals` to the shared test helper.

- [ ] **Step 4: Run tests and build**

Run: `cd web && npm test -- --run src/pages/operator/Overview.test.ts src/composables/useLiveStatus.test.ts && npm run build`

Expected: PASS in both themes.

- [ ] **Step 5: Commit**

```bash
git add web/src/api.ts web/src/types/api.ts web/src/pages/operator web/src/pages/PoolOverview.vue web/src/components/TelemetryChart.vue web/src/components/OperatorNav.vue web/src/composables/useLiveStatus* web/src/composables/useEvents.ts web/src/router.ts web/src/test/helpers.ts
git commit -m "feat(web): add operator overview and activity"
```

## Phase 3 — Safe Operations and Alerts

### Task 10: Enrich payout and validator-action APIs

**Files:**
- Create: `internal/api/operator_operations_handlers.go`
- Create: `internal/api/operator_operations_handlers_test.go`
- Modify: `internal/api/operator_handlers.go`
- Modify: `internal/pool/lifecycle.go`
- Modify: `internal/pool/pool.go`
- Modify: `internal/pool/confirmation.go`
- Modify: `internal/pool/confirmation_test.go`
- Modify: `sql/queries.sql`
- Regenerate: `internal/db/queries.sql.go`

**Interfaces:**
- Produces paginated `GET /api/operator/payouts?status=&cursor=&limit=` and `GET /api/operator/actions?status=&cursor=&limit=`.
- Produces `POST /api/operator/payouts/{id}/retry`, `POST /api/operator/actions`, and `POST /api/operator/actions/{id}/cancel`.
- Action states: `requested`, `processing`, `submitted`, `confirmed`, `failed`, `cancelled`.

- [ ] **Step 1: Write failing transition tests**

```go
func TestCancelValidatorActionOnlyWhileRequested(t *testing.T) {
    a, cookie, _ := operatorTestAPI(t)
    requestedID := seedAction(t, a.queries, "retire", "requested")
    assertPostStatus(t, a, cookie, fmt.Sprintf("/api/operator/actions/%d/cancel", requestedID), http.StatusOK)
    processingID := seedAction(t, a.queries, "deactivate", "processing")
    assertPostStatus(t, a, cookie, fmt.Sprintf("/api/operator/actions/%d/cancel", processingID), http.StatusConflict)
}
```

- [ ] **Step 2: Run and verify the new routes fail**

Run: `go test ./internal/api -run 'Test.*(Payout|ValidatorAction|Action)' -v`

Expected: FAIL with 404.

- [ ] **Step 3: Implement conditional transitions and audit events**

Use conditional updates:

```sql
-- name: CancelRequestedValidatorAction :one
UPDATE validator_actions SET state='cancelled', updated_at=CURRENT_TIMESTAMP
WHERE id=? AND state='requested' RETURNING id;
```

When a payout transaction fails confirmation, mark its linked payslips `failed` instead of silently resetting them; normal automatic payouts still process `pending` rows. Retry only failed payslips with no active transaction, reset the selected payout group to pending in a DB transaction, and record actor/correlation ID. The daemon changes validator actions from requested to processing before signing, then submitted or failed; each tick also checks submitted action hashes and moves them to confirmed or failed. Reject duplicate outstanding actions with 409. Preserve old deactivate/retire routes as wrappers over the new action service.

- [ ] **Step 4: Regenerate and test**

Run: `sqlc generate && go test ./internal/api ./internal/pool -v`

Expected: PASS with invalid transitions covered.

- [ ] **Step 5: Commit**

```bash
git add internal/api/operator_operations_handlers.go internal/api/operator_operations_handlers_test.go internal/api/operator_handlers.go internal/pool/lifecycle.go internal/pool/pool.go internal/pool/confirmation.go internal/pool/confirmation_test.go sql/queries.sql internal/db/models.go internal/db/queries.sql.go
git commit -m "feat(api): add safe operator action queues"
```

### Task 11: Record alert deliveries and expose redacted alert management

**Files:**
- Modify: `internal/notifier/notifier.go`
- Create: `internal/notifier/notifier_test.go`
- Create: `internal/api/operator_alert_handlers.go`
- Create: `internal/api/operator_alert_handlers_test.go`
- Modify: `internal/api/api.go`
- Modify: `internal/api/operator_handlers.go`
- Modify: `internal/config/config.go`

**Interfaces:**
- `Notifier.Send(ctx context.Context, alert Alert) []DeliveryResult`.
- `Alert` fields: `Level`, `Type`, `Title`, `Message`, `CorrelationID`.
- API exposes only configured/enabled/destination hint/latest status, never tokens, through `GET /api/operator/alerts`, `POST /api/operator/alerts/{channel}/test`, and `GET /api/operator/alerts/deliveries?cursor=&limit=`.

- [ ] **Step 1: Write failing delivery/redaction tests**

```go
func TestNotifierRecordsWebhookFailureWithoutSecret(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 500) }))
    defer server.Close()
    recorder := &fakeDeliveryRecorder{}
    n := New(&config.Config{AlertWebhookURL: server.URL + "?token=secret"}, WithDeliveryRecorder(recorder))
    got := n.Send(context.Background(), Alert{Level: "error", Type: "test", Title: "Test alert", Message: "Delivery check"})
    if got[0].State != "failed" || strings.Contains(recorder.last.Destination, "secret") { t.Fatalf("%+v", got) }
}
```

- [ ] **Step 2: Run and verify signatures/routes fail**

Run: `go test ./internal/notifier ./internal/api -run 'Test.*Alert' -v`

Expected: FAIL because the new signature and routes do not exist.

- [ ] **Step 3: Implement results, persistence, redacted API, and rate limit**

Treat only HTTP 2xx as success. Close response bodies, cap error text, redact URL query/user-info, and persist every attempt. Report email as `unavailable` until a real sender exists. Add `GET /api/operator/alerts`, `POST /api/operator/alerts/{channel}/test`, and `GET /api/operator/alerts/deliveries`. Permit one test per channel per 30 seconds.

- [ ] **Step 4: Run focused and full tests**

Run: `go test ./internal/notifier ./internal/api -v && go test ./...`

Expected: PASS; no response body contains a secret.

- [ ] **Step 5: Commit**

```bash
git add internal/notifier/notifier.go internal/notifier/notifier_test.go internal/api/operator_alert_handlers.go internal/api/operator_alert_handlers_test.go internal/api/api.go internal/api/operator_handlers.go internal/config/config.go
git commit -m "feat(ops): add redacted alert delivery management"
```

### Task 12: Build Operations and Alerts pages

**Files:**
- Create: `web/src/pages/operator/Operations.vue`
- Create: `web/src/pages/operator/Alerts.vue`
- Create: `web/src/pages/operator/Operations.test.ts`
- Create: `web/src/pages/operator/Alerts.test.ts`
- Modify: `web/src/types/api.ts`
- Modify: `web/src/router.ts`
- Modify: `web/src/components/OperatorNav.vue`
- Modify: `web/src/test/helpers.ts`

**Interfaces:**
- Operations consumes Task 10 responses and `HoldConfirmButton`.
- Alerts consumes Task 11 redacted responses.

- [ ] **Step 1: Write failing safety/secret tests**

```ts
it('requires hold confirmation before posting retire', async () => {
  const post = vi.spyOn(api, 'apiPost').mockResolvedValue({ state: 'requested' })
  const wrapper = mount(Operations, { global: operatorTestGlobals })
  await wrapper.get('[data-action="retire"]').trigger('click')
  expect(post).not.toHaveBeenCalled()
  wrapper.findComponent(HoldConfirmButton).vm.$emit('confirm')
  await flushPromises()
  expect(post).toHaveBeenCalledWith('/api/operator/actions', { action: 'retire' })
})

it('never renders an alert token', async () => {
  mockAlertResponse({ telegram: { configured: true, destination_hint: 'chat …4821' } })
  const wrapper = mount(Alerts)
  await flushPromises()
  expect(wrapper.text().toLowerCase()).not.toContain('secret')
})
```

- [ ] **Step 2: Run and verify pages are missing**

Run: `cd web && npm test -- --run src/pages/operator/Operations.test.ts src/pages/operator/Alerts.test.ts`

Expected: FAIL resolving pages.

- [ ] **Step 3: Implement queue-first operations and alert history**

Show attention-required rows first, with status/address/amount filters and inline detail. Put validator actions last in a danger section with review copy and hold confirmation. Alerts shows configured/missing/unavailable, a test action and result, and paginated deliveries.

- [ ] **Step 4: Run tests and build**

Run: `cd web && npm test -- --run src/pages/operator/Operations.test.ts src/pages/operator/Alerts.test.ts && npm run build`

Expected: PASS in light and dark themes.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/operator/Operations* web/src/pages/operator/Alerts* web/src/types/api.ts web/src/router.ts web/src/components/OperatorNav.vue web/src/test/helpers.ts
git commit -m "feat(web): add operator operations and alerts"
```

## Phase 4 — Setup and Configuration

### Task 13: Add secret-file loading and an atomic revision store

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Create: `internal/configstore/store.go`
- Create: `internal/configstore/store_test.go`
- Modify: `internal/ops/recorder.go`
- Modify: `cmd/main.go`
- Modify: `sql/queries.sql`
- Regenerate: `internal/db/queries.sql.go`

**Interfaces:**
- Produces `config.Editable` and `config.Redacted` with no secret fields.
- Produces `config.LoadOptional(path string) (*Config, bool, error)` for signer-free API use and `config.LoadDaemon(path string) (*Config, error)` for daemon-only secret loading.
- Produces `configstore.Store.Validate`, `Save`, `ListRevisions`, and `Restore`.
- `Save(ctx, actor, expectedHash, editable)` returns `Revision{ID, Hash, RestartRequired}`.
- Produces `config.EditableHash(editable Editable) string`; the daemon places this hash in every later heartbeat.

- [ ] **Step 1: Write failing boundary/atomicity tests**

```go
func TestStoreRejectsStaleHashWithoutChangingFile(t *testing.T) {
    store := newTestStore(t)
    before := readConfig(t, store.Path())
    _, err := store.Save(context.Background(), testAddr, "stale", validEditable())
    if !errors.Is(err, ErrRevisionConflict) { t.Fatalf("%v", err) }
    if !bytes.Equal(before, readConfig(t, store.Path())) { t.Fatal("file changed") }
}

func TestRedactedConfigNeverContainsSecrets(t *testing.T) {
    cfg := validConfig()
    cfg.PrivateKey = "private-value-7f8c"
    cfg.SessionSecret = "session-value-5a2d"
    cfg.AlertTelegramToken = "telegram-value-9b1e"
    got, _ := json.Marshal(config.Redact(cfg))
    for _, secret := range []string{cfg.PrivateKey, cfg.SessionSecret, cfg.AlertTelegramToken} {
        if bytes.Contains(got, []byte(secret)) { t.Fatal(string(got)) }
    }
}
```

- [ ] **Step 2: Run and verify types/store are missing**

Run: `go test ./internal/config ./internal/configstore -v`

Expected: FAIL for missing package/functions.

- [ ] **Step 3: Implement paths, validation, redaction, and atomic save**

Read `CONFIG_FILE`, defaulting to `config.json`. `LoadOptional` returns `configured=false` only for a missing file, never malformed JSON, and decodes through a signer-free disk struct that omits legacy `private_key`. `LoadDaemon` alone reads `POOL_PRIVATE_KEY_FILE` and may read a legacy on-disk key only to support a documented migration warning; the API never materializes it. `Editable` contains RPC URL, network, fee wallet/percentage, payout mode, minimum payout, auto-reactivation, API address, validator address, operator addresses, metrics address, alert channel enablement/destinations, pool name, pool description, contact URL, and disclosure text. Validate network, HTTP(S) RPC URL, fee `[0,1)`, payout mode, minimum payout, Nimiq addresses, and non-empty public pool name.

Write candidate JSON to a sibling 0600 temporary file, sync, rename, then sync the directory. Persist only redacted before/after values and SHA-256 hashes. Reject `expectedHash` mismatch. A failed write must preserve the original file. Pass `EditableHash` into the recorder created by `cmd/main.go` so new heartbeats confirm the active revision.

- [ ] **Step 4: Regenerate and test**

Run: `sqlc generate && go test ./internal/config ./internal/configstore -v`

Expected: PASS including simulated write/rename failures.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go internal/configstore internal/ops/recorder.go cmd/main.go sql/queries.sql internal/db/queries.sql.go
git commit -m "feat(config): add safe revisioned settings store"
```

### Task 14: Add setup-only API mode and settings endpoints

**Files:**
- Create: `internal/api/setup_auth.go`
- Create: `internal/api/setup_auth_test.go`
- Create: `internal/api/setup_handlers.go`
- Create: `internal/api/setup_handlers_test.go`
- Create: `internal/api/operator_settings_handlers.go`
- Create: `internal/api/operator_settings_handlers_test.go`
- Modify: `internal/api/api.go`
- Modify: `internal/api/pool_handlers.go`
- Modify: `internal/api/pool_handlers_test.go`
- Modify: `cmd/api/main.go`

**Interfaces:**
- `api.New` accepts `WithSetup(tokenHash, configStore)` and `WithConfigStore(configStore)` options.
- Setup cookie: HttpOnly, SameSite Strict, path `/api/setup`, 30-minute lifetime.
- Produces `POST /api/setup/session`, `GET /api/setup/status`, `POST /api/setup/check-rpc`, `POST /api/setup/validate`, and `POST /api/setup/complete`.
- Produces `GET /api/operator/settings`, `POST /api/operator/settings/validate`, `PUT /api/operator/settings`, `GET /api/operator/settings/revisions`, and `POST /api/operator/settings/revisions/{id}/restore`.

- [ ] **Step 1: Write failing lockout/concurrency tests**

```go
func TestSetupTokenExchangeDisabledAfterCompletion(t *testing.T) {
    a := setupTestAPI(t, WithSetupComplete(true))
    rec := postJSON(t, a.Mux(), "/api/setup/session", map[string]string{"token": "bootstrap"}, nil)
    if rec.Code != http.StatusNotFound { t.Fatalf("%d", rec.Code) }
}

func TestSettingsSaveRequiresCurrentHash(t *testing.T) {
    a, cookie := configuredOperatorAPI(t)
    rec := putJSON(t, a.Mux(), "/api/operator/settings", validEditableBody("stale"), cookie)
    if rec.Code != http.StatusConflict { t.Fatalf("%d: %s", rec.Code, rec.Body.String()) }
}
```

- [ ] **Step 2: Run and verify missing routes fail**

Run: `go test ./internal/api -run 'Test(Setup|Settings)' -v`

Expected: FAIL with 404 or missing options.

- [ ] **Step 3: Implement bootstrap and configured modes**

When config is missing, `cmd/api` binds to `POOL_API_ADDR` or `:8080`, builds only static/setup routes, and does not construct RPC. Read and hash `POOL_SETUP_TOKEN_FILE` at startup, compare submitted tokens in constant time, issue a random server-side session, consume setup access after completion, and rate-limit failures by remote IP. Configured mode reads `POOL_SESSION_SECRET_FILE`. `check-rpc` uses signer-free RPC; `validate` returns field errors; `complete` atomically saves config and disables setup.

Configured settings endpoints return active hash, pending revision, daemon-reported secret presence, validation results, revision list, save, and restore. Extend the public pool response with `pool_name`, `pool_description`, `contact_url`, and `disclosure`, using `GoPool` and empty optional strings for installations upgraded before they save a profile. Never serialize `Config` directly.

- [ ] **Step 4: Run API/full tests**

Run: `go test ./internal/api -run 'Test(Setup|Settings)' -v && go test ./...`

Expected: PASS; setup routes are 404 after completion and normal operator routes are unavailable during setup.

- [ ] **Step 5: Commit**

```bash
git add internal/api/setup_auth.go internal/api/setup_auth_test.go internal/api/setup_handlers.go internal/api/setup_handlers_test.go internal/api/operator_settings_handlers.go internal/api/operator_settings_handlers_test.go internal/api/api.go internal/api/pool_handlers.go internal/api/pool_handlers_test.go cmd/api/main.go
git commit -m "feat(api): add secure setup and settings modes"
```

### Task 15: Build setup assistant and Settings page

**Files:**
- Create: `web/src/pages/setup/Setup.vue`
- Create: `web/src/pages/setup/steps/SystemCheck.vue`
- Create: `web/src/pages/setup/steps/ValidatorIdentity.vue`
- Create: `web/src/pages/setup/steps/PoolEconomics.vue`
- Create: `web/src/pages/setup/steps/PublicProfile.vue`
- Create: `web/src/pages/setup/steps/AccessAlerts.vue`
- Create: `web/src/pages/setup/steps/ReviewLaunch.vue`
- Create: `web/src/pages/setup/Setup.test.ts`
- Create: `web/src/pages/operator/Settings.vue`
- Create: `web/src/pages/operator/Settings.test.ts`
- Modify: `web/src/router.ts`
- Modify: `web/src/types/api.ts`
- Modify: `web/src/test/helpers.ts`

**Interfaces:**
- Setup owns typed `SetupDraft` and advances only after server validation.
- Settings sends `{ expected_hash, settings: EditableConfig }`.
- Secret state is only `configured`, `missing`, or `pending verification`.

- [ ] **Step 1: Write failing progression/review tests**

```ts
it('cannot advance economics with an invalid fee', async () => {
  mockSetupValidation({ field_errors: { pool_fee_percentage: 'Must be below 100%' } })
  const wrapper = mount(Setup, { global: setupTestGlobals })
  await goToEconomics(wrapper)
  await wrapper.get('[name="pool_fee_percentage"]').setValue('100')
  await wrapper.get('[data-next]').trigger('click')
  expect(wrapper.text()).toContain('Must be below 100%')
  expect(wrapper.attributes('data-step')).toBe('economics')
})

it('shows before/after before saving settings', async () => {
  const wrapper = mount(Settings, { global: operatorTestGlobals })
  await loadSettings(wrapper)
  await wrapper.get('[name="min_payout_nim"]').setValue('25')
  await wrapper.get('[data-review]').trigger('click')
  expect(wrapper.get('[data-review-panel]').text()).toContain('10 NIM → 25 NIM')
})
```

- [ ] **Step 2: Run and verify pages are missing**

Run: `cd web && npm test -- --run src/pages/setup/Setup.test.ts src/pages/operator/Settings.test.ts`

Expected: FAIL resolving pages.

- [ ] **Step 3: Implement six steps and revision UX**

Use one question group per step, visible progress, back navigation, server field errors, and a redacted review. Validator identity links to deployment docs and remains pending until daemon heartbeat. After completion, show restart commands and poll readiness without claiming active state. Settings groups economics, profile, access, and alerts; saving/restoring uses the same review panel and shows restart required until heartbeat config hash matches.

- [ ] **Step 4: Run tests and build**

Run: `cd web && npm test -- --run src/pages/setup/Setup.test.ts src/pages/operator/Settings.test.ts && npm run build`

Expected: PASS in both themes.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/setup web/src/pages/operator/Settings* web/src/router.ts web/src/types/api.ts web/src/test/helpers.ts
git commit -m "feat(web): add setup assistant and settings"
```

## Phase 5 — Delivery and Release Verification

### Task 16: Update deployment contracts and documentation

**Files:**
- Create: `deployments/verify_contract.py`
- Modify: `deployments/docker-compose.yml`
- Modify: `deployments/docker-stack.yml`
- Modify: `devlab/docker-compose.pool.yml`
- Modify: `README.md`
- Modify: `deployments/SWARM.md`
- Modify: `config/config.json-example`
- Modify: `config.mainnet.json`
- Modify: `config.testnet.json`
- Modify: `.gitignore`

**Interfaces:**
- Daemon receives `/run/secrets/gopool_validator_key` and `POOL_PRIVATE_KEY_FILE`.
- API receives no validator secret; API gets `POOL_SETUP_TOKEN_FILE` and `POOL_SESSION_SECRET_FILE` as API-only secrets.
- Both share DB/config directory; API config mount is writable and daemon mount is read-only.

- [ ] **Step 1: Write a failing deployment-contract check**

```python
import json, subprocess, sys

rendered = subprocess.check_output([
    'docker', 'compose', '-f', 'deployments/docker-compose.yml',
    'config', '--format', 'json',
], text=True)
services = json.loads(rendered)['services']
api = json.dumps(services['gopool-api'])
daemon = json.dumps(services['gopool'])
assert 'POOL_PRIVATE_KEY' not in api and 'gopool_validator_key' not in api
assert 'POOL_PRIVATE_KEY_FILE' in daemon and 'gopool_validator_key' in daemon
assert 'gopool_setup_token' in api and 'gopool_session_secret' in api
```

- [ ] **Step 2: Run and verify the current contract fails**

Run: `python3 deployments/verify_contract.py`

Expected: FAIL because current services share a read-only config and no daemon-only secret.

- [ ] **Step 3: Implement separation and copy-paste instructions**

Update Compose, Swarm, and devlab with distinct mount modes, daemon-only validator secret, API-only setup/session secrets, and health checks. Add `.secrets/` to `.gitignore`. Document these commands plus token removal, rotation, revision restore, and failed readiness:

```bash
# Local Compose
install -d -m 700 .secrets
openssl rand -hex 32 > .secrets/setup-token
openssl rand -hex 32 > .secrets/session-secret
install -m 600 /dev/stdin .secrets/validator-key <<<'<validator-private-key-hex>'
docker compose -f deployments/docker-compose.yml up -d

# Docker Swarm
openssl rand -hex 32 | docker secret create gopool_setup_token -
openssl rand -hex 32 | docker secret create gopool_session_secret -
printf '%s' '<validator-private-key-hex>' | docker secret create gopool_validator_key -
docker stack deploy -c deployments/docker-stack.yml gopool
```

- [ ] **Step 4: Validate rendered deployment files**

Run:

```bash
python3 deployments/verify_contract.py
docker compose -f deployments/docker-compose.yml config >/dev/null
docker compose -f devlab/docker-compose.yaml -f devlab/docker-compose.pool.yml config >/dev/null
```

Expected: PASS with no API key mount.

- [ ] **Step 5: Commit**

```bash
git add .gitignore deployments/verify_contract.py deployments/docker-compose.yml deployments/docker-stack.yml deployments/SWARM.md devlab/docker-compose.pool.yml README.md config/config.json-example config.mainnet.json config.testnet.json
git commit -m "docs: add secure GoPool first-run deployment"
```

### Task 17: Run the complete verification gate

**Files:**
- Create: `web/src/accessibility.test.ts`

**Interfaces:**
- Produces passing Go binaries/tests and `internal/api/webdist`.
- Does not deploy or push; those require a separate explicit request.

- [ ] **Step 1: Add cross-cutting accessibility tests**

```ts
it.each(['light', 'dark'])('keeps status text and keyboard focus in %s', async theme => {
  document.documentElement.dataset.theme = theme
  const wrapper = mount(OperatorOverview, { global: operatorTestGlobals })
  await flushPromises()
  expect(wrapper.get('[role="status"]').text()).toMatch(/healthy|attention|offline/i)
  const first = wrapper.find('a,button,input')
  await first.trigger('focus')
  expect(document.activeElement).toBe(first.element)
})
```

- [ ] **Step 2: Run every automated gate**

Run:

```bash
go fmt ./...
go test ./...
go vet ./...
cd web
npm test -- --run
npm run build
cd ..
docker build -t gopool:verification .
python3 deployments/verify_contract.py
```

Expected: every command exits 0 and frontend output is embedded under `internal/api/webdist`.

- [ ] **Step 3: Commit the cross-cutting automated test**

```bash
git add web/src/accessibility.test.ts
git commit -m "test(web): cover themes and keyboard status"
```

- [ ] **Step 4: Run browser verification against local devnet**

Verify at 1440px, 768px, and 390px: all setup steps and restart readiness; public overview/performance/epoch/lookup/Hub flows; all operator pages; light/dark/system/reduced-motion/keyboard modes; SSE disconnect/reconnect with stale data; failed RPC, empty data, expired session, invalid settings, and rejected actions. If a scenario fails, stop this task, return to the task that owns the failing component, add a focused regression test there, fix it, rerun that task's gate, then restart Task 17 Step 2.

- [ ] **Step 5: Rerun the final read-only regression gates**

Run: `go test ./... && go vet ./... && cd web && npm test -- --run && npm run build`

Expected: PASS with no TypeScript, Go, theme, responsive, or state regressions.

## Phase Review Gates

After each phase:

1. Run its focused tests and full Go/Vue build gates.
2. Review the diff for accidental inclusion of pre-existing user changes.
3. Verify no secrets occur in fixtures, logs, snapshots, revisions, or bundles.
4. Confirm the UI never claims queued config/action state is active.
5. Confirm public routes and automatic payouts remain usable.

Do not begin the next phase until the current phase is passing and reviewed.
