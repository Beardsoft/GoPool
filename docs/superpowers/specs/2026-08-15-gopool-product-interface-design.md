# GoPool Product Interface and Operator Console Design

**Status:** Approved for implementation planning
**Date:** 2026-08-15

## Goal

Turn GoPool from a daemon with a basic status UI into the easiest credible way
for another person to launch and operate a public Nimiq staking pool.

The product must serve two audiences without mixing their jobs:

- Prospective and existing stakers need a trustworthy public pool surface,
  transparent performance data, and a direct view of their own stake and
  payouts.
- Pool operators need guided first-run setup, health and exception monitoring,
  payout and validator operations, structured activity, alert management, and
  safe configuration controls.

The selected visual direction is Nimiq-native and premium. Public and operator
pages share one design system, but each has its own information architecture.

## Product principles

1. Explain before asking. Setup and financial controls state their effect in
   plain language before asking for a decision.
2. Health before detail. The operator overview answers whether the pool is
   healthy before displaying raw telemetry.
3. Exceptions before logs. Items that require action are more prominent than
   general activity.
4. Review before mutation. Every configuration or validator change is
   validated, summarized, confirmed, and audited.
5. One theme at a time. A page is wholly light or wholly dark; it never mixes a
   dark application shell with unrelated white sections.
6. Preserve custody boundaries. The validator private key never enters the
   browser or the API process.

## Scope

This design includes:

- A browser-based first-run setup assistant.
- A redesigned public pool and staker experience.
- A full authenticated operator console.
- Read-oriented and operator APIs required by the console.
- Structured operational history, health snapshots, alert delivery history,
  and configuration revisions.
- Safe configuration editing for non-secret values.
- Complete light and dark themes built from the Nimiq UI Kit.
- Responsive, accessible, and failure-aware interface states.

This design does not include:

- Multi-validator or multi-pool tenancy.
- Browser entry, storage, display, or recovery of the validator private key.
- A raw terminal or arbitrary command execution in the browser.
- Editing secret values such as private keys, session secrets, or notification
  tokens in the UI.
- Replacing Prometheus/Grafana for infrastructure-level monitoring.
- Making GoPool a wallet or constructing staking transactions for stakers.

## Current baseline

GoPool currently has separate daemon and API processes sharing SQLite. The API
serves a Vue 3 application and exposes public pool, epoch, reward, staker, SSE,
and limited operator endpoints. Operator health currently consists of daemon
height, chain head, and stuck payslips; validator actions can queue deactivate
or retire requests.

The current uncommitted product work is treated as the baseline and must be
preserved. It includes onboarding, staker history, reward data, SSE events,
alerts, metrics, deployment work, dark-mode work, and edits across the daemon,
API, and frontend. Implementation must begin by reconciling this baseline and
must not discard unrelated user changes.

The frontend currently does not compile because `App.vue` imports `ref` and
`onMounted` from `vue-router` instead of `vue`. Fixing that defect is part of
the implementation foundation.

## Product architecture

### First-run mode

If no valid completed configuration exists, the API starts in setup-only mode.
The normal public application and operator APIs remain unavailable.

Setup access uses a one-time `POOL_SETUP_TOKEN` supplied through deployment
configuration. The browser exchanges it at `POST /api/setup/session` for a
short-lived, HttpOnly, SameSite setup cookie. The token is never persisted to
SQLite or returned by an API. Setup endpoints are disabled after completion.

The setup assistant has six steps:

1. **System check:** database access, config-file writability, RPC reachability,
   and network identity. Secret verification is explicitly pending until the
   first daemon heartbeat because the API cannot access the key mount.
2. **Validator identity:** collect the public validator address and explain how
   to mount the key secret. After restart, the daemon derives the address and
   refuses to enter normal operation if it differs from the configured public
   address.
3. **Pool economics:** pool fee, payout mode, minimum payout, and
   auto-reactivation with plain-language consequences and validation.
4. **Public profile:** pool name, short description, operator/contact URL, and
   optional location or disclosure text.
5. **Access and alerts:** one or more Hub-authenticated operator addresses,
   enabled alert channels, and non-secret delivery destinations. Secret tokens
   remain deployment-managed.
6. **Review and launch:** show the complete redacted configuration, validation
   results, secret/readiness checks, restart instructions, and expected public
   URL.

The setup completion writes configuration atomically, records the initial
redacted revision, and marks setup complete. It does not attempt to restart
containers. After the operator restarts the services, the daemon publishes its
heartbeat and validator-address verification result for the readiness screen.

README and deployment documentation must provide copy-paste instructions for
creating the setup token, mounting the validator key as a Docker secret or
read-only secret file through `POOL_PRIVATE_KEY_FILE`, and restarting the
services. The daemon reads that file; the API container does not mount it.

### Configured mode

Once configured, one deployment provides two surfaces:

- **Public pool:** Overview, Performance, Staker lookup, and My stake.
- **Operator console:** Overview, Operations, Activity, Alerts, and Settings.

Public navigation does not advertise destructive operator tools. Operators can
reach `/operator` directly or through a restrained operator sign-in link. The
operator console includes a clear link back to the public pool.

### Access levels

- **Public:** pool profile, current status, epochs, reward history, public
  staker lookup, and public readiness status.
- **Staker session:** Nimiq Hub authentication resolving `/api/me` to the
  session address.
- **Operator session:** Hub authentication whose address equals the validator
  address or appears in the configured operator allowlist.
- **Setup session:** temporary setup-token session available only before setup
  completion.

## Visual system

The implementation uses the local Nimiq UI Kit as its design source of truth.
It reuses the current Vue and CSS stack rather than adding a second component
framework.

### Tokens and type

- Primary ink and dark surface: Nimiq blue `#1F2348`.
- Actions and informational states: Nimiq light blue `#0582CA`, using
  `#0CA6FE` on dark surfaces.
- Healthy and completed states: Nimiq green `#21BCA5`.
- Warnings and value emphasis: Nimiq gold `#E9B213`.
- Destructive and failed states: Nimiq red `#D94432`, using `#FF5C48` on dark
  surfaces.
- Primary colored surfaces use the official bottom-right Nimiq radial
  gradients and their darkened hover variants.
- Neutral text uses the Nimiq blue opacity ladder rather than an unrelated gray
  palette.
- Muli is used for interface text. Fira Mono is used for Nimiq addresses,
  hashes, exact timestamps, and dense numeric telemetry.
- Spacing remains on an 8px grid. Cards use the Nimiq 10px radius; primary
  actions use pill geometry. The application keeps a 16px root size if needed
  and translates kit measurements instead of changing every existing `rem`.

### Components and identity

- NIM amounts are stored as integer luna and rendered as NIM using
  `1 NIM = 100000 luna`, locale-aware grouping, and at most five decimals.
- Staker and operator addresses use deterministic Nimiq identicons, canonical
  grouped addresses, and copy controls.
- Use the Nimiq icon sprite for navigation, status, copy, settings, download,
  login, warning, and confirmation icons.
- Prefer existing local components. Do not add `@nimiq/vue-components` solely
  for this redesign; reproduce the required token-driven patterns locally.
- High-impact operations use the Nimiq `LongPressButton` interaction pattern
  or a local equivalent with accessible keyboard confirmation.

### Theme behavior

The first visit follows `prefers-color-scheme`. A manual light/dark selection
overrides the system preference and persists locally. Public and operator pages
use the same active theme. Switching theme changes the whole application;
individual sections never force the opposite theme.

## Public and staker experience

### Public overview

The homepage combines trust and live proof:

- A concise promise: transparent, non-custodial Nimiq staking.
- A live pool-health card with current epoch and progress.
- Delegated stake, rewards paid, staker count, and pool fee.
- An immediate Nimiq-address lookup entry point.
- A short explanation of how rewards and fees work.
- Recent verified pool activity and a path to detailed performance.

The current standalone onboarding and staker-lookup concepts collapse into one
canonical address flow. There should not be two nearly identical pages asking
for an address.

### Performance

Performance combines the existing epoch list, epoch details, and reward
history into an understandable analytics surface:

- Reward totals and pool fees over time.
- Epoch and batch filters.
- Current and historical delegated stake and staker counts.
- Accessible tables under every chart.
- Links from chart points or table rows to epoch detail.

Charts use lightweight SVG/CSS or an already-present chart dependency. Do not
add a visualization library solely for decorative charts.

### Staker lookup and My stake

Both views use the same position component:

- Identicon and canonical address.
- Current stake and pool share.
- Cumulative and recent rewards.
- Payout history with transaction status and hash.
- Per-epoch stake history.
- CSV export.

Public lookup takes an explicit address. My stake signs in through Nimiq Hub
and resolves the same component to the authenticated address. Empty history is
explained without implying a failure.

## Operator console

### Navigation

The console uses a compact top application header plus a persistent contextual
tab bar: Overview, Operations, Activity, Alerts, and Settings. This avoids a
generic oversized dashboard sidebar and remains usable on narrow screens.

### Overview

The operator's daily home uses this hierarchy:

1. Overall pool status and the number of passing readiness checks.
2. Chain lag, pending payouts, last tick duration, and payout-wallet runway.
3. A needs-attention queue for failed actions, stuck payouts, RPC issues,
   validator state, and alert delivery failures.
4. Validator state, election/epoch context, live stake, staker count, and next
   checkpoint estimate.
5. A selected operational trend, defaulting to chain lag over 24 hours.
6. Recent structured activity.

When nothing needs action, the attention area explicitly says so. Destructive
actions are not placed on this overview.

### Operations

Operations contains:

- Payout summary and filterable payout/payslip queue.
- Requested, processing, submitted, confirmed, failed, and cancelled states.
- Transaction hashes and affected recipients.
- Safe retry for eligible failed payouts and cancellation of validator actions
  that have not begun processing.
- Validator lifecycle state and deactivate/retire actions.
- A review summary followed by hold-to-confirm for validator lifecycle changes.

Automatic payout behavior remains the default. The console makes the queue and
failures visible; it does not require manual approval for every payout.

### Activity

Activity is a structured operational log, not a raw terminal:

- Severity, category, source, summary, timestamp, actor, and correlation ID.
- Filters for validator, epoch, payout, alert, configuration, authentication,
  and system events.
- Expandable redacted context.
- Links to related payouts, actions, config revisions, or epochs.
- CSV/JSON export of the current filtered result.

### Alerts

Alerts shows:

- Enabled channels and whether their required deployment secret is available.
- Non-secret destinations where applicable.
- Conditions covered by the existing notifier.
- A test-delivery action with a clear result.
- Delivery history and failures.

Secret tokens are never returned by the API. The UI displays only configured,
missing, or invalid status.

### Settings

Editable settings include pool fee, payout mode, minimum payout,
auto-reactivation, public pool profile, enabled alert channels, non-secret
destinations, and operator addresses.

Private keys, session secrets, and alert tokens are masked and read-only. The
UI links to deployment instructions for changing them.

Saving settings performs server-side validation, shows a redacted before/after
review, writes the config atomically, records a revision, and marks the console
as restart-required. The interface never claims changes are active before a
new daemon heartbeat reports the expected config revision. A prior valid
revision can be restored through the same review and restart flow.

## API design

All operator endpoints require an operator session. Setup endpoints require a
setup session and are unavailable after setup completion.

### Setup

- `POST /api/setup/session` — exchange the one-time token for a setup cookie.
- `GET /api/setup/status` — setup state and non-secret readiness checks.
- `POST /api/setup/check-rpc` — validate RPC URL and network.
- `POST /api/setup/validate` — validate a redacted candidate configuration.
- `POST /api/setup/complete` — atomically write config and initial revision.

### Operator overview and telemetry

- `GET /api/operator/overview` — overall status, readiness checks, attention
  items, validator summary, payout summary, and recent activity.
- `GET /api/operator/telemetry?metric=&from=&to=&bucket=` — bounded historical
  telemetry series.
- `GET /api/operator/readiness` — individual health checks and last heartbeat.

### Operations and activity

- `GET /api/operator/payouts?status=&cursor=&limit=` — payout and payslip queue.
- `POST /api/operator/payouts/{id}/retry` — retry an eligible failed payout.
- `GET /api/operator/actions?status=&cursor=&limit=` — validator action queue.
- `POST /api/operator/actions` — queue an allowed validator action.
- `POST /api/operator/actions/{id}/cancel` — cancel a still-requested action.
- `GET /api/operator/activity?severity=&category=&from=&to=&cursor=&limit=` —
  structured operational events.
- `GET /api/operator/activity/export?...` — filtered CSV or JSON export.

Existing deactivate and retire routes may remain temporarily as compatibility
wrappers around the generic action service, then be removed only in a separate
breaking change.

### Alerts and settings

- `GET /api/operator/alerts` — redacted channel configuration and delivery
  status.
- `POST /api/operator/alerts/{channel}/test` — send a test through the current
  channel configuration.
- `GET /api/operator/alerts/deliveries?cursor=&limit=` — delivery history.
- `GET /api/operator/settings` — active, pending, and secret-presence state.
- `POST /api/operator/settings/validate` — validate proposed non-secret values.
- `PUT /api/operator/settings` — atomically save a validated revision.
- `GET /api/operator/settings/revisions` — redacted revision history.
- `POST /api/operator/settings/revisions/{id}/restore` — stage a previous valid
  revision.

Mutation endpoints reject stale clients using a revision identifier or ETag.
Errors return a stable machine code plus a plain-language message.

## Persistence

Add forward-only SQLite migrations for these concepts:

### `runtime_status`

One current daemon heartbeat row containing timestamp, daemon version, config
revision, derived validator address, validator state, last processed height,
chain head, last tick duration, RPC status, and readiness error.

### `health_snapshots`

Minute-level samples for trend charts: timestamp, chain head, processed height,
tick duration, validator state, live stake, staker count, pending/stuck payout
counts and amounts, payout-wallet balance, and RPC health. Retain 30 days by
default and query through bounded buckets.

### `operator_events`

Structured events with severity, category, source, event type, summary,
redacted JSON context, actor address, correlation ID, and timestamp. This is
the UI's operational log. Apply bounded retention to general events while
preserving audit-specific records in their dedicated tables.

### `alert_deliveries`

Channel, alert type, redacted destination, state, response/error summary,
attempted time, and correlation ID.

### `config_revisions`

Revision ID, actor address, redacted before/after documents, validation state,
write state, created time, activated time, and the config hash expected from the
next daemon heartbeat.

### Validator action enrichment

Extend `validator_actions` with requested-by, requested-at, updated-at,
explicit state, error summary, and correlation ID while preserving existing
rows and compatibility queries.

## Runtime and data flow

1. The daemon remains the only process that holds the validator key and signs
   transactions.
2. The daemon writes its heartbeat, snapshots, and operational events to the
   shared SQLite database.
3. The API reads pool and operator data, validates operator changes, writes
   config revisions, and queues actions. It never signs transactions.
4. The daemon consumes queued actions, updates their states, and writes related
   events.
5. SSE publishes new health, activity, payout, and action events. The frontend
   reconciles each event with its current view and periodically refreshes
   authoritative snapshots.
6. Config changes take effect only after service restart. The new heartbeat's
   config hash confirms activation.

## Error and state behavior

- Loading states use layout-shaped skeletons.
- Refreshes preserve last-known data and mark it stale instead of clearing the
  page.
- SSE disconnects show “Live updates paused” and retry automatically.
- RPC, database, config, notifier, and authentication errors use distinct
  operator-facing messages and stable API codes.
- Session expiry returns through Hub and restores the intended route.
- Missing setup or deployment secrets link to exact documentation.
- Empty attention, payout, activity, alert, epoch, and staker states explain
  what will appear and whether action is needed.
- Optimistic UI is not used for configuration or on-chain actions. The UI shows
  the server-returned queued state until a later heartbeat or action update
  confirms progress.

## Responsive and accessibility behavior

- Desktop content is constrained to a readable maximum width.
- Operator tabs become a horizontally scrollable or compact menu on phones.
- Metric grids collapse to two and then one column.
- Dense tables provide responsive row/card representations without hiding
  status, amount, address, or action information.
- Every control is keyboard accessible with a visible focus state.
- Hold-to-confirm has an accessible keyboard alternative and confirmation text.
- Status meaning is conveyed by text and icon, not color alone.
- Motion honors `prefers-reduced-motion`.
- Charts have textual summaries and equivalent data tables.
- Address and hash truncation never removes access to the complete copyable
  value.

## Testing and verification

### Backend

- Table-driven handler tests for every setup and operator endpoint.
- Setup-token expiry, reuse, completion lockout, and auth-boundary tests.
- Config validation, redaction, optimistic concurrency, atomic write, failed
  write, restore, and activation-hash tests.
- Migration tests against existing schema data.
- Runtime heartbeat, snapshot retention, event recording, alert delivery, and
  action state-transition tests.
- Explicit tests proving APIs never return configured secrets.

### Frontend

- `vue-tsc --noEmit` and Vite production build.
- Component tests for theme preference, amount formatting, address display,
  stale/live state, setup validation, settings review, and hold confirmation.
- Route and role tests for public, staker, operator, and setup modes.
- Loading, empty, error, disconnected, and session-expired states.
- Browser verification at desktop, tablet, and phone widths in both themes.
- Keyboard, focus, reduced-motion, semantic form, and contrast checks.

### End-to-end

- Fresh deployment through setup completion and post-restart readiness.
- Public pool browsing and address lookup.
- Hub staker login resolving the same position view.
- Hub operator login, failed-payout investigation, alert test, settings change,
  restart-required state, and activation confirmation.
- Deactivate/retire review, hold confirmation, queue processing, and failure
  recovery using a test environment only.

## Implementation phases

The product is one design but implementation should land in independently
verifiable phases:

1. **Foundation and public experience:** fix the current build, establish
   Nimiq tokens/components/themes, consolidate public navigation and address
   flows, and redesign public/staker pages.
2. **Operator observability:** persistence migrations, runtime heartbeat,
   snapshots, structured activity, overview/telemetry APIs, and the Overview
   and Activity pages.
3. **Safe operations and alerts:** enriched action/payout states, Operations
   page, alert configuration/status, test delivery, and delivery history.
4. **Setup and configuration:** setup-only mode, bootstrap session, setup
   assistant, config revisions, settings validation/write/restore, activation
   confirmation, and deployment documentation.
5. **Release verification:** full backend/frontend test gates, both-theme and
   responsive browser checks, fresh-install rehearsal, upgrade rehearsal, and
   production packaging verification.

Each phase must preserve a buildable application and must not claim a queued,
written, or restarted state is active until runtime evidence confirms it.

## Acceptance criteria

- A new operator can reach a readiness-checked configuration using the browser
  assistant and deployment documentation without editing JSON by hand.
- The validator private key never enters the browser or API process.
- The public homepage explains the pool and displays current proof of operation
  without exposing operator controls.
- A staker can find their current position, history, payouts, and export from
  one canonical flow.
- The operator overview answers health, attention, validator, payout, trend,
  and recent-activity questions without opening raw logs.
- Operators can investigate payout/action state, configure and test alerts,
  and safely stage non-secret settings.
- All mutations are validated, confirmed, audited, and reconciled with daemon
  state.
- Light and dark themes are complete, Nimiq-native, persistent, responsive, and
  accessible.
- The Vue production build and all new backend, frontend, and end-to-end gates
  pass against the final implementation.
