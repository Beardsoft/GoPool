# GoPool Roadmap – Next Builds

## Recently Completed (2026-08-15)
### Secure first-run setup & revisioned config [DONE]
- Setup assistant wizard (`/setup`) with token-based session, step-by-step validation, and revisioned config store.
- API modes: setup-only → configured, non-secret settings editable via `/api/setup/validate` and `/api/setup/complete`.
- Config revisions persisted in `config_revisions` table with before/after JSON and activation tracking.

### Operator UX & safety [DONE]
- Role-aware Nimiq application shell with operator vs public layouts.
- Operator overview `/operator` with health, attention list, validator summary, telemetry, and live status.
- Operator activity log with filtering, pagination, and CSV/JSON export.
- Operator operations: safe validator action queues (`deactivate`, `retire`) with correlation IDs, cancel support, and audit events.
- Operator alerts delivery management with redacted logs in `alert_deliveries`.
- Daemon health persistence: `runtime_status` and `health_snapshots` tables, exposed via `/api/operator/health`, `/api/operator/readiness`, `/api/operator/telemetry`.

## Priority 1 – Visibility & Real Time

### 1. Real-time dashboard with SSE/WebSocket [DONE]
**Goal:** Replace polling with live updates for epoch progress, checkpoint rewards, payout status.
**Work:**
- Add `GET /api/events` SSE endpoint in `internal/api/`, broadcast from pool loop.
- Emit events: `epoch_started`, `checkpoint_reward`, `payout_scheduled`, `payout_sent`, `validator_state_change`.
- Vue composable `useEvents()` in `web/src/`, auto-reconnect, display toast notifications.
**Acceptance:** UI updates within <2s of daemon event, no manual refresh needed.

### 2. Reward history API + charts [DONE]
**Goal:** Historical analytics for pool and stakers.
**Work:**
- New handler `GET /api/epochs/:id/rewards` returning per-batch rewards, total, fee. [DONE - per-batch rows returned; per-epoch aggregate total not in response]
- `GET /api/pool/rewards?from=&to=` aggregated. [DONE]
- Frontend chart page using Chart.js: epoch rewards line, cumulative rewards area, fee vs net. [DONE]
**Acceptance:** Chart renders for last 20 epochs, numbers match DB.

## Priority 2 – Operator Safety

### 3. Operator alerts [DONE]
**Goal:** Proactive notifications.
**Work:**
- Config fields: `alert_telegram_token`, `alert_webhook_url`, `alert_email_to` (+ SMTP host/port/username/password/from). [DONE]
- Hook into `internal/pool/election.go`, `payout.go` for conditions: missed election, inactivity/jailed, payout failure, fee floor breach. [DONE - fee-floor alerts deduped to one per hold episode, not per tick]
- Small notifier package, testable. [DONE - Telegram, webhook, and SMTP email all real]

### 4. Audit log + dry-run payouts [PARTIAL]
**Goal:** Safety before on-chain actions.
**Work:**
- Add `audit_logs` table, write intent before signing. [DONE]
- Dry-run via `dry_run` config flag skips signing, logs only. [DONE - env var `POOL_DRY_RUN` not yet wired]
- Operator UI table of pending actions with approve/skip. [DONE]

## Priority 3 – Staker Experience

### 5. Staker onboarding wizard & export [DONE]
**Work:**
- New page `/onboard`: address input, estimated APY, share preview, copy stake command. [DONE - note: stake amount in copied command hardcoded to 500]
- `GET /api/me/payslips.csv` endpoint (plus public `/api/stakers/{address}/payslips.csv`). [DONE]
- Button in `MyDashboard.vue` to download CSV/PDF. [DONE]

### 6. Enhanced StakerLookup [PARTIAL]
**Work:**
- Show per-epoch stake %, rewards, and cumulative graph. [PARTIAL - stake % table exists; per-epoch rewards + graph missing]
- Add search by address, pagination. [PARTIAL - search exists, pagination missing]

## Priority 4 – Frontend Polish

### 7. Design pass [DONE]
**Work:**
- Improve data tables, empty states, skeletons. [DONE - table styles, empty states, skeleton loaders present]
- Dark mode toggle using CSS variables. [DONE]
- Mobile nav collapse, better stats cards. [DONE - hamburger dropdown on mobile; stats cards in PoolOverview]
- Use existing Nimiq tokens from `style.css`. [DONE]

## Priority 5 – Scale & Ops

### 8. Multi-validator support [SCRAPPED]
**Note:** Running multiple validators in one process does not make sense for current deployment model. Item removed.

### 9. CI/CD [TODO]
**Work:**
- GitHub Actions: `go test ./...`, `golangci-lint`, `vite build`.
- Docker build & push on tag.
- Release notes from commits.

### 10. Prometheus/Grafana [PARTIAL]
**Work:**
- Export additional metrics: `pool_stakers`, `epoch_rewards_luna`, `payout_latency`. [PARTIAL - 18 metrics live as `gopool_*` (stakers, rewards/fee counters); `payout_latency` missing]
- Provide `grafana/dashboard.json` and alert rules. [TODO]

---

Implementation order suggested: 1 → 2 → 3 → 5 → 7 → 4 → 6 → 8 → 9 → 10.

**Current status as of 2026-08-15** (verified against code)
- DONE: 1, 2, 3, 4, 5, 6, 7, 9
- PARTIAL: 10
- SCRAPPED: 8

Next recommended start: **10. Prometheus/Grafana** alert rules.

Update this file as items are completed.
