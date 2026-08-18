# Epoch Election and Slot Stats Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. REQUIRED: superpowers:test-driven-development for every task.

**Goal:** Show whether the pool validator is in this epoch’s active set and how many slots it holds, as two tiles on the operator overview.

**Architecture:** Pure Hamilton slot math over `getActiveValidators` + policy `slots`. Daemon caches the last good snapshot on the 30s gauge tick and writes it on every `runtime_status` heartbeat. Overview remains a DB read. UI adds two metric tiles and switches the strip to a 3-column grid.

**Tech Stack:** Go 1.25, SQLite/sqlc, Vue 3 + Vitest, existing `nimiq-go` RPC (`GetActiveValidators`, `Client.Call` for `getPolicyConstants`).

**Design:** `docs/plans/2026-08-18-epoch-slot-stats-design.md`

---

### Task 1: Hamilton slot share

**Files:**
- Create: `internal/pool/slots.go`
- Test: `internal/pool/slots_test.go`

**Step 1: Write the failing test**

```go
package pool

import (
	"testing"

	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/NimMiniApps/nimiq-go/rpc"
)

func TestSlotShareNotInSet(t *testing.T) {
	vs := []rpc.Validator{{Address: "NQ01 AAAA", Balance: 100}}
	elected, count := slotShare("NQ02 BBBB", vs, 512)
	if elected || count != 0 {
		t.Fatalf("elected=%v count=%d", elected, count)
	}
}

func TestSlotShareEmptySet(t *testing.T) {
	elected, count := slotShare("NQ01 AAAA", nil, 512)
	if elected || count != 0 {
		t.Fatalf("elected=%v count=%d", elected, count)
	}
}

func TestSlotShareIgnoresAddressSpaces(t *testing.T) {
	vs := []rpc.Validator{{Address: "NQ01 AAAA BBBB", Balance: nimiq.Luna(512)}}
	elected, count := slotShare("NQ01AAAABBBB", vs, 512)
	if !elected || count != 512 {
		t.Fatalf("elected=%v count=%d", elected, count)
	}
}

func TestSlotShareLargestRemainder(t *testing.T) {
	// 3 validators, 5 slots. Floors are 2,1,1 leftover 1 → highest remainder gets it.
	vs := []rpc.Validator{
		{Address: "NQ01 A", Balance: 50}, // 50*5/100 = 2.50 → 2
		{Address: "NQ02 B", Balance: 30}, // 1.50 → 1
		{Address: "NQ03 C", Balance: 20}, // 1.00 → 1
	}
	_, a := slotShare("NQ01 A", vs, 5)
	_, b := slotShare("NQ02 B", vs, 5)
	_, c := slotShare("NQ03 C", vs, 5)
	if a+b+c != 5 {
		t.Fatalf("sum=%d want 5 (a=%d b=%d c=%d)", a+b+c, a, b, c)
	}
	if a != 3 || b != 1 || c != 1 {
		t.Fatalf("a=%d b=%d c=%d want 3,1,1", a, b, c)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/pool -run TestSlotShare -count=1`
Expected: FAIL, `slotShare` undefined

**Step 3: Write minimal implementation**

`internal/pool/slots.go`:

- `normalizeAddr` strips spaces
- `slotShare(addr, validators, slots)` returns `(elected bool, count uint32)`
- Hamilton: `floor(stake * slots / total)` then give leftover 1s to highest `stake*slots % total`, tie-break by normalized address ascending
- Not in set / total 0 / slots 0 → `(false, 0)`

**Step 4: Run test to verify it passes**

Run: `go test ./internal/pool -run TestSlotShare -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/pool/slots.go internal/pool/slots_test.go
git commit -m "Add epoch slot-share math for the active validator set."
```

---

### Task 2: Persist epoch participation on runtime_status

**Files:**
- Create: `internal/db/migrations/007_epoch_participation.sql`
- Modify: `schema/scheme.sql` (`runtime_status` columns)
- Modify: `sql/queries.sql` (`UpsertRuntimeStatus`, `GetRuntimeStatus`)
- Modify: `internal/db/migrate_test.go` (assert new columns)
- Modify: `internal/ops/types.go`, `internal/ops/recorder.go`
- Modify: `internal/api/operator_test_helpers_test.go` only if compile breaks

**Step 1: Write the failing test**

In `internal/db/migrate_test.go`, after migrate, assert columns exist:

```go
func assertColumnExists(t *testing.T, db *sql.DB, table, column string) {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == column {
			if notnull != 0 {
				t.Fatalf("%s.%s should be nullable", table, column)
			}
			return
		}
	}
	t.Fatalf("missing column %s.%s", table, column)
}
```

Call it for `epoch_number`, `epoch_elected`, `slot_count`, `slots_total` on `runtime_status` in `TestMigrateFreshAndLegacyDatabases`.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/db -run TestMigrateFreshAndLegacyDatabases -count=1`
Expected: FAIL, missing columns

**Step 3: Write minimal implementation**

`007_epoch_participation.sql`:

```sql
ALTER TABLE runtime_status ADD COLUMN epoch_number INTEGER;
ALTER TABLE runtime_status ADD COLUMN epoch_elected INTEGER;
ALTER TABLE runtime_status ADD COLUMN slot_count INTEGER;
ALTER TABLE runtime_status ADD COLUMN slots_total INTEGER;
```

Add the same four nullable columns to `schema/scheme.sql`.

Update `UpsertRuntimeStatus` / `GetRuntimeStatus` to include them. On conflict, keep last snapshot when the new value is NULL:

```sql
epoch_number=COALESCE(excluded.epoch_number, runtime_status.epoch_number),
epoch_elected=COALESCE(excluded.epoch_elected, runtime_status.epoch_elected),
slot_count=COALESCE(excluded.slot_count, runtime_status.slot_count),
slots_total=COALESCE(excluded.slots_total, runtime_status.slots_total)
```

Run: `/home/maestro/go/bin/sqlc generate`

Extend `ops.Heartbeat` with `EpochNumber, EpochElected, SlotCount, SlotsTotal sql.NullInt64` and pass them through `RecordHeartbeat`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/db ./internal/ops ./internal/api -count=1`
Expected: PASS (helpers compile with zero-value NullInt64 = SQL NULL)

**Step 5: Commit**

```bash
git add internal/db schema sql internal/ops internal/api
git commit -m "Store epoch election and slot snapshot on runtime status."
```

---

### Task 3: Daemon refreshes active-set membership

**Files:**
- Modify: `internal/pool/pool.go`
- Modify: `internal/metrics/metrics.go`
- Test: `internal/pool/pool_test.go` or new `internal/pool/epoch_participation_test.go`

**Step 1: Write the failing test**

Table-driven test around a small helper used by the tick (keep RPC out of the unit if possible):

```go
func TestApplyActiveSetSnapshot(t *testing.T) {
	var got epochParticipation
	vs := []rpc.Validator{{Address: "NQ01 A", Balance: 100}}
	applyActiveSet(&got, "NQ01 A", vs, 2, 512)
	if !got.valid || got.epoch != 2 || !got.elected || got.slotCount != 512 {
		t.Fatalf("%+v", got)
	}
	applyActiveSet(&got, "NQ99 Z", vs, 3, 512)
	if !got.valid || got.elected || got.slotCount != 0 || got.epoch != 3 {
		t.Fatalf("unelected %+v", got)
	}
}
```

Do not clear `got` on a simulated RPC error; add `TestKeepLastParticipationOnRPCFailure` that only updates when `applyActiveSet` is called.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/pool -run 'TestApplyActiveSet|TestKeepLast' -count=1`
Expected: FAIL, types missing

**Step 3: Write minimal implementation**

On `Manager`, keep `epochPart epochParticipation` (`valid, epoch, elected, slotCount, slotsTotal`).

On the existing ~30s gauge refresh in `observeTickGauges`:

1. `Call(ctx, "getPolicyConstants", nil, &struct{ Slots uint32 `json:"slots"` })` — cache `slots` after first success
2. `GetActiveValidators`
3. `applyActiveSet` using `epochAt(m.policy, head)` and `slotShare`
4. Set gauges `gopool_epoch_elected` (1/0), `gopool_validator_slots`, `gopool_slots_total`
5. On RPC error: increment `RPCErrors` (`active_validators` / `policy`), leave `epochPart` unchanged

`recordHeartbeat` copies `epochPart` into `ops.Heartbeat` NullInt64s when `valid`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/pool ./internal/metrics -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/pool internal/metrics
git commit -m "Snapshot active-set membership and slot count on the daemon tick."
```

---

### Task 4: Overview API exposes epoch_participation

**Files:**
- Modify: `internal/api/operator_overview_handlers.go`
- Modify: `internal/api/operator_overview_handlers_test.go`

**Step 1: Write the failing test**

Extend `operatorOverviewResponseTest` with:

```go
EpochParticipation *struct {
	Epoch      *int64 `json:"epoch"`
	Elected    *bool  `json:"elected"`
	SlotCount  *int64 `json:"slot_count"`
	SlotsTotal *int64 `json:"slots_total"`
} `json:"epoch_participation"`
```

Existing tests: all four fields JSON `null` when seed has NULLs.

New test: seed NullInt64 values (epoch 2, elected 1, slots 12, total 512) and assert `elected=true`, `slot_count=12`. Status stays `healthy` when not elected (seed elected=0, no attention events).

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api -run TestOperatorOverview -count=1`
Expected: FAIL, field missing or always null

**Step 3: Write minimal implementation**

Add `EpochParticipation epochParticipationJSON` to `operatorOverviewResponse`.

```go
type epochParticipationJSON struct {
	Epoch      *int64 `json:"epoch"`
	Elected    *bool  `json:"elected"`
	SlotCount  *int64 `json:"slot_count"`
	SlotsTotal *int64 `json:"slots_total"`
}
```

Map from `GetRuntimeStatus`. `epoch_elected` 0/1 → bool pointer only when Valid. Do not change overall status from this field.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api -run TestOperatorOverview -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api
git commit -m "Expose epoch participation on the operator overview API."
```

---

### Task 5: Overview tiles

**Files:**
- Modify: `web/src/types/api.ts`
- Modify: `web/src/test/helpers.ts` (`overviewFixture`)
- Modify: `web/src/pages/operator/Overview.vue`
- Modify: `web/src/pages/operator/Overview.test.ts`

**Step 1: Write the failing test**

```ts
it('shows elected status and slot count on the metrics strip', async () => {
  mockFetch('/api/operator/overview', overviewFixture({
    epoch_participation: { epoch: 2, elected: true, slot_count: 12, slots_total: 512 },
  }))
  mockFetch('/api/pool', { current_epoch: 2, epoch_status: 'in_progress', num_stakers: 1, total_stake_luna: 0, total_rewards_luna: 0, pool_fee_percentage: 0.01 })
  const wrapper = mount(Overview, { global: { ...operatorTestGlobals, stubs: { RouterLink: { template: '<a href="#"><slot /></a>' } } } })
  await flushPromises()
  const metrics = wrapper.get('.metrics').text()
  expect(metrics).toContain('Elected')
  expect(metrics).toContain('Epoch 2')
  expect(metrics).toContain('12')
  expect(metrics).toContain('of 512')
})

it('shows placeholders before the first epoch snapshot', async () => {
  mockFetch('/api/operator/overview', overviewFixture())
  mockFetch('/api/pool', { current_epoch: 2, epoch_status: 'in_progress', num_stakers: 1, total_stake_luna: 0, total_rewards_luna: 0, pool_fee_percentage: 0.01 })
  const wrapper = mount(Overview, { global: { ...operatorTestGlobals, stubs: { RouterLink: { template: '<a href="#"><slot /></a>' } } } })
  await flushPromises()
  expect(wrapper.get('.metrics').text()).toMatch(/—/)
  expect(wrapper.get('.metrics').text()).not.toContain('Not elected')
})
```

**Step 2: Run test to verify it fails**

Run: `npm test -- --run src/pages/operator/Overview.test.ts`
Expected: FAIL (copy missing)

**Step 3: Write minimal implementation**

`OperatorOverview.epoch_participation` with nullable fields. Fixture defaults all to `null`.

In `Overview.vue` metrics, prepend:

```html
<article>
  <p>This epoch</p>
  <strong>{{ epochElectedLabel }}</strong>
  <small>Epoch {{ overview.epoch_participation?.epoch ?? '—' }}</small>
</article>
<article>
  <p>Slots</p>
  <strong>{{ overview.epoch_participation?.slot_count ?? '—' }}</strong>
  <small>of {{ overview.epoch_participation?.slots_total ?? '—' }}</small>
</article>
```

`epochElectedLabel`: `null` → `—`; `true` → `Elected`; `false` → `Not elected`.

CSS: `.metrics { grid-template-columns: repeat(3, 1fr); }` and fix the 900px/620px border rules so six cells still divide cleanly (3×2, then 2×3, then 1×6). First row of three keeps right borders; wrap rows get a top/bottom divider like the existing 4-col wrap.

**Step 4: Run test to verify it passes**

Run: `npm test -- --run src/pages/operator/Overview.test.ts src/accessibility.test.ts src/layouts/OperatorLayout.test.ts`
Expected: PASS

**Step 5: Commit**

```bash
git add web
git commit -m "Show epoch election and slot count on the operator overview."
```

---

## Done when

- Elected vs not elected is visible without reading activity events
- Slot count is `N of slots_total` from the active set, not validator account state
- RPC blips and daemon restarts keep the last snapshot (COALESCE + in-memory cache)
- Health/attention behavior is unchanged
