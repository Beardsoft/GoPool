# Nimiq Session UX and Identicons Design

## Goal

One Nimiq wallet login should unlock the whole experience: operators (any
address configured via `validator_address` or `operator_addresses`) get the
operator menu, every logged-in staker sees their own stake performance in
"Find my stake", and addresses render with deterministic Nimiq identicons.
"Find my stake" stays fully usable without login.

## What already exists

Wallet auth is complete and untouched by this work:

- `POST /api/auth/challenge` + `POST /api/auth/verify`
  (`internal/api/auth.go`) — Hub `signMessage` challenge, HMAC-SHA256
  self-contained `gopool_session` cookie, 24 h TTL.
- `requireOperator` (`internal/api/operator_handlers.go`) already accepts the
  validator address and every `operator_addresses` member.
- `loginWithHub()` (`web/src/hub.ts`) already performs the sign-in.

This work is session visibility, navigation, and presentation.

## Session endpoints

- `GET /api/session` (requireSession) returns
  `{ "address": string, "operator": bool }`. `operator` reuses the existing
  `isOperatorAllowed` check. 401 when signed out.
- `POST /api/auth/logout` clears the `gopool_session` cookie. No other state
  exists to revoke (sessions are stateless HMAC tokens).

## Frontend session composable

`web/src/composables/useSession.ts` fetches `/api/session` once
(module-level cached promise, no duplicate requests) and exposes
`{ signedIn, address, operator, login(), logout() }`. `login()` wraps the
existing `loginWithHub()`; on success the cache is invalidated and refetched.

## Header

`AppHeader.vue`:

- Signed out: a "Sign in with Nimiq" button; the Operator link stays visible
  (leads to the existing OperatorLayout sign-in prompt).
- Signed in: an identicon + short-address chip. The Operator link renders only
  when `operator` is true.

## Identicons

- New dependency `@nimiq/identicons` (official Nimiq package, SVG data-URL
  identicons; no TypeScript types, so a local module declaration is added).
- New `web/src/components/ui/Identicon.vue`: `address` + `size` props, renders
  the deterministic identicon as an `<img>` from `Identicons.toDataUrl`;
  renders nothing for invalid addresses.
- Built into `AddressIdentity.vue` (covers operator Overview and any future
  reuse). Added at the raw-render sites: `StakerLookup.vue`,
  `MyDashboard.vue`, `operator/Operations.vue` payouts, `EpochDetail.vue`
  staker table.

## Find my stake

`StakerLookup.vue` with a session and no `:address` route param auto-loads the
session address, shows its identicon and a "Your stake" badge. The lookup field
remains for arbitrary addresses. Without a session the page behaves exactly as
today: anonymous, no wallet required.

## Out of scope

Per-user dashboards, multi-account support, changes to the auth mechanism, and
any login requirement for public stake lookup.

## Tests

- Go: handler tests for `GET /api/session` (signed out, staker, operator) and
  `POST /api/auth/logout` (cookie cleared), following existing handler test
  patterns.
- Vitest: `useSession` composable with mocked fetch (signed-in, signed-out,
  login refresh); existing `OperatorLayout.test.ts` hub mock stays valid.
- Gates: `go test ./...`, `go vet`, `npm test`, `npm run build`.
