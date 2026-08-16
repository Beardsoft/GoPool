# Nimiq Session UX and Identicons Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** One Nimiq wallet login unlocks the whole app — operators get the operator menu, stakers see their own stake in "Find my stake" — and every address renders with a Nimiq identicon.

**Architecture:** Wallet auth already exists (Hub `signMessage` → HMAC cookie; `requireOperator` already accepts configured operator addresses). This plan adds two small backend endpoints (`GET /api/session`, `POST /api/auth/logout`), a shared `useSession` composable that the header and stake lookup consume, and an `Identicon` component embedded in the two shared address-display components (`ExplorerLink`, `AddressIdentity`) so every address-render site gets it for free.

**Tech Stack:** Go `net/http` (Go 1.22+ method-pattern mux), Vue 3 + Vite + vitest (happy-dom), `@nimiq/identicons` (SVG data-URL identicons, official Nimiq package).

**Context the executor needs:**

- Repo: Go backend at repo root, Vue frontend in `web/`.
- Session cookie name is `gopool_session` (`internal/api/auth.go:21`). `requireSession` middleware puts the authenticated `nimiq.Address` into context via `addressFromContext(r.Context())` (`auth.go:167`).
- `isOperatorAllowed(addr string)` (`internal/api/operator_handlers.go:32`) checks the comma-split `cfg.OperatorAddresses`; `cfg.ValidatorAddress` is checked separately. Both are on `*config.Config`.
- Test helper `operatorTestAPI(t)` (`internal/api/operator_handlers_test.go:47`) returns `(*API, operatorCookie, stakerCookie)` — the API has `cfg.ValidatorAddress = testAddr` where `testAddr = "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"` (`internal/api/staker_handlers_test.go:18`); `stakerCookie` is a signed-in non-operator.
- `writeJSON(w, status, body)` and `writeError(w, status, msg)` are the response helpers.
- Frontend fetch wrapper: `apiGet/apiPost/apiPut` in `web/src/api.ts` (all `credentials: 'include'`, throw `{status, code, message}` on non-2xx).
- Test helper `mockFetch(path, body, status=200)` in `web/src/test/helpers.ts` stubs global `fetch` with an exact-path map (module-level, persists across tests in a file). `resetExplorerForTests()` and `loadNetwork()` are in `web/src/composables/useExplorer.ts`.
- `shortAddress(addr)` in `web/src/utils/format.ts` renders `NQ20 TSB0 … D3MA 859E`.
- `loginWithHub()` in `web/src/hub.ts` performs the existing challenge/verify flow; after Task 7 it must only be called through `useSession().login()`.
- Nimiq addresses are 36 chars: `N` + network letter (`Q` mainnet / `K` testnet) + 2 chars + 8 groups of 4, e.g. `NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E`.
- `@nimiq/identicons` (v1.6.2) is installed in `web/` (verify: `cd web && npm ls @nimiq/identicons`). It has no TypeScript types. The ES module export is a class with static async methods; `Identicons.toDataUrl(text)` returns `data:image/svg+xml;base64,…` suitable for `<img src>`. It works on any string (hashes it), so invalid addresses must be filtered before calling it.
- Gates after every frontend task: `cd web && npx vitest run` and `npm run build`. Gates after Go tasks: `go test ./internal/api/ && go vet ./internal/api/`.

---

### Task 1: `GET /api/session` endpoint

**Files:**
- Modify: `internal/api/auth.go` (route in `registerAuthRoutes` ~line 80-83; new handler)
- Test: `internal/api/auth_test.go`

**Step 1: Write the failing test**

Append to `internal/api/auth_test.go`:

```go
func TestSessionEndpoint(t *testing.T) {
	a, operatorCookie, stakerCookie := operatorTestAPI(t)

	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/session", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("signed out: status = %d, want 401", rec.Code)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.AddCookie(operatorCookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("operator: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var body sessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Address != testAddr || !body.Operator {
		t.Errorf("operator session = %+v, want address %s operator true", body, testAddr)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.AddCookie(stakerCookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("staker: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	body = sessionResponse{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Operator {
		t.Errorf("staker session = %+v, want operator false", body)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestSessionEndpoint -v`
Expected: compile FAIL — `sessionResponse` undefined.

**Step 3: Write minimal implementation**

In `internal/api/auth.go`, add the type and handler near the other auth handlers:

```go
type sessionResponse struct {
	Address  string `json:"address"`
	Operator bool   `json:"operator"`
}

func (a *API) handleSession(w http.ResponseWriter, r *http.Request) {
	addr, _ := addressFromContext(r.Context())
	writeJSON(w, http.StatusOK, sessionResponse{
		Address:  addr.String(),
		Operator: addr.String() == a.cfg.ValidatorAddress || a.isOperatorAllowed(addr.String()),
	})
}
```

Register it in `registerAuthRoutes`:

```go
func (a *API) registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/challenge", a.handleAuthChallenge)
	mux.HandleFunc("POST /api/auth/verify", a.handleAuthVerify)
	mux.HandleFunc("GET /api/session", a.requireSession(a.handleSession))
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestSessionEndpoint -v && go vet ./internal/api/`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/api/auth.go internal/api/auth_test.go
git commit -m "feat: add GET /api/session returning address and operator flag"
```

---

### Task 2: `POST /api/auth/logout` endpoint

**Files:**
- Modify: `internal/api/auth.go` (route + handler)
- Test: `internal/api/auth_test.go`

**Step 1: Write the failing test**

Append to `internal/api/auth_test.go`:

```go
func TestAuthLogoutClearsCookie(t *testing.T) {
	a := &API{cfg: &config.Config{SessionSecret: "test-secret"}}
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || cookies[0].MaxAge != -1 {
		t.Fatalf("cookies = %+v, want expired %s cookie", cookies, sessionCookieName)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestAuthLogoutClearsCookie -v`
Expected: FAIL — 404 (route not registered).

**Step 3: Write minimal implementation**

In `internal/api/auth.go`:

```go
func (a *API) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
```

Register in `registerAuthRoutes`: `mux.HandleFunc("POST /api/auth/logout", a.handleAuthLogout)`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ && go vet ./internal/api/`
Expected: PASS (whole package).

**Step 5: Commit**

```bash
git add internal/api/auth.go internal/api/auth_test.go
git commit -m "feat: add POST /api/auth/logout to clear the session cookie"
```

---

### Task 3: `Identicon` component

**Files:**
- Create: `web/src/types/identicons.d.ts`
- Create: `web/src/components/ui/Identicon.vue`
- Test: `web/src/components/ui/Identicon.test.ts`

**Step 1: Write the failing test**

Create `web/src/components/ui/Identicon.test.ts`:

```ts
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import Identicon from './Identicon.vue'

const ADDR = 'NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE'

describe('Identicon', () => {
  it('renders a data-url identicon for a valid address', async () => {
    const wrapper = mount(Identicon, { props: { address: ADDR } })
    await flushPromises()
    const img = wrapper.get('img')
    expect((img.element as HTMLImageElement).src.startsWith('data:image/svg+xml;base64,')).toBe(true)
  })

  it('renders nothing for an invalid address', async () => {
    const wrapper = mount(Identicon, { props: { address: 'not-an-address' } })
    await flushPromises()
    expect(wrapper.find('img').exists()).toBe(false)
  })
})
```

**Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/components/ui/Identicon.test.ts`
Expected: FAIL — cannot find module `./Identicon.vue`.

**Step 3: Write the type declaration and component**

Create `web/src/types/identicons.d.ts` (tsconfig `include: ["src"]` picks it up):

```ts
declare module '@nimiq/identicons' {
  const Identicons: {
    svg(text: string): Promise<string>
    toDataUrl(text: string): Promise<string>
    render(text: string, element: HTMLElement): Promise<void>
    image(text: string): Promise<HTMLImageElement>
    placeholder(color?: string, strokeWidth?: number): string
    placeholderToDataUrl(color?: string, strokeWidth?: number): string
    renderPlaceholder(element: HTMLElement, color?: string, strokeWidth?: number): void
  }
  export default Identicons
}
```

Create `web/src/components/ui/Identicon.vue`:

```vue
<script setup lang="ts">
import { ref, watch } from 'vue'
import Identicons from '@nimiq/identicons'

const props = withDefaults(defineProps<{ address: string; size?: number }>(), { size: 20 })
const src = ref('')

watch(
  () => props.address,
  async (address) => {
    const value = address.trim()
    if (!/^N[QK][0-9A-Z]{2}( [0-9A-Z]{4}){8}$/.test(value)) {
      src.value = ''
      return
    }
    src.value = await Identicons.toDataUrl(value)
  },
  { immediate: true },
)
</script>

<template>
  <img v-if="src" class="identicon" :src="src" :width="size" :height="size" :alt="`Identicon for ${address}`" />
</template>

<style scoped>
.identicon {
  border-radius: 5px;
  flex-shrink: 0;
}
</style>
```

**Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/components/ui/Identicon.test.ts && npm run build`
Expected: PASS + clean build.

**Step 5: Commit**

```bash
git add web/src/types/identicons.d.ts web/src/components/ui/Identicon.vue web/src/components/ui/Identicon.test.ts web/package.json web/package-lock.json
git commit -m "feat: add Identicon component backed by @nimiq/identicons"
```

---

### Task 4: Identicons in shared address components

Every page renders addresses through `ExplorerLink kind="account"` (StakerLookup, MyDashboard, Operations payouts, EpochDetail staker table) or `AddressIdentity` (operator Overview), so both components get the identicon and all sites are covered.

**Files:**
- Modify: `web/src/components/ui/ExplorerLink.vue`
- Modify: `web/src/components/ui/AddressIdentity.vue`
- Test: `web/src/components/ui/ui.test.ts`

**Step 1: Write the failing test**

In `web/src/components/ui/ui.test.ts`, add imports at the top:

```ts
import { flushPromises } from '@vue/test-utils'
import { beforeEach } from 'vitest'
import ExplorerLink from './ExplorerLink.vue'
import AddressIdentity from './AddressIdentity.vue'
import { mockFetch } from '../../test/helpers'
import { loadNetwork, resetExplorerForTests } from '../../composables/useExplorer'
```

Add a `beforeEach(() => { resetExplorerForTests() })` inside the describe, then new cases:

```ts
it('shows an identicon next to account links', async () => {
  mockFetch('/api/pool', { network: 'test-albatross' })
  await loadNetwork()
  const wrapper = mount(ExplorerLink, {
    props: { kind: 'account', value: 'NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE' },
  })
  await flushPromises()
  expect(wrapper.find('img.identicon').exists()).toBe(true)
})

it('shows no identicon for transaction links', async () => {
  mockFetch('/api/pool', { network: 'test-albatross' })
  await loadNetwork()
  const wrapper = mount(ExplorerLink, { props: { kind: 'transaction', value: 'ab'.repeat(32) } })
  await flushPromises()
  expect(wrapper.find('img.identicon').exists()).toBe(false)
})

it('shows an identicon in AddressIdentity', async () => {
  const wrapper = mount(AddressIdentity, { props: { address: 'NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE' } })
  await flushPromises()
  expect(wrapper.find('img.identicon').exists()).toBe(true)
})
```

**Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/components/ui/ui.test.ts`
Expected: the three new cases FAIL (no `img.identicon`).

**Step 3: Update both components**

`ExplorerLink.vue` — add `import Identicon from './Identicon.vue'`; replace the template with:

```vue
<template>
  <span class="explorer-link">
    <Identicon v-if="kind === 'account'" :address="value" />
    <a v-if="href" :href="href" target="_blank" rel="noopener noreferrer" :title="title ?? value">{{ label ?? value }}</a>
    <span v-else class="explorer-mono" :title="title ?? value">{{ display }}</span>
  </span>
</template>
```

Add to its `<style scoped>`:

```css
.explorer-link {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
```

`AddressIdentity.vue` — add `import Identicon from './Identicon.vue'`; in the template, place `<Identicon :address="address" />` as the first child inside `<span class="address-identity">`.

**Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run && npm run build`
Expected: full suite PASS (existing tests select `a`/`.explorer-mono` which still exist inside the wrapper span) + clean build.

**Step 5: Commit**

```bash
git add web/src/components/ui/ExplorerLink.vue web/src/components/ui/AddressIdentity.vue web/src/components/ui/ui.test.ts
git commit -m "feat: render identicons in ExplorerLink and AddressIdentity"
```

---

### Task 5: `useSession` composable

**Files:**
- Create: `web/src/composables/useSession.ts`
- Test: `web/src/composables/useSession.test.ts`

**Step 1: Write the failing test**

Create `web/src/composables/useSession.test.ts`:

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useSession, resetSessionCache } from './useSession'
import { mockFetch } from '../test/helpers'

const ADDR = 'NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE'

vi.mock('../hub', () => ({ loginWithHub: vi.fn() }))

const settle = () => new Promise((resolve) => setTimeout(resolve, 0))

describe('useSession', () => {
  beforeEach(() => {
    resetSessionCache()
    ;(vi.importActual<typeof import('../hub')>('../hub')).then?.(() => {})
    vi.mocked((await import('../hub')).loginWithHub).mockReset()
  })

  it('reports signed out when the session endpoint rejects', async () => {
    mockFetch('/api/session', { error: 'not logged in' }, 401)
    const { signedIn, address } = useSession()
    await settle()
    expect(signedIn.value).toBe(false)
    expect(address.value).toBe('')
  })

  it('exposes the session address and operator flag', async () => {
    mockFetch('/api/session', { address: ADDR, operator: true })
    const { signedIn, address, operator } = useSession()
    await settle()
    expect(signedIn.value).toBe(true)
    expect(address.value).toBe(ADDR)
    expect(operator.value).toBe(true)
  })

  it('login signs in via Hub and re-fetches the session', async () => {
    const { loginWithHub } = await import('../hub')
    vi.mocked(loginWithHub).mockResolvedValue({ address: ADDR })
    mockFetch('/api/session', { address: ADDR, operator: false })
    const { signedIn, login } = useSession()
    await login()
    expect(loginWithHub).toHaveBeenCalledTimes(1)
    expect(signedIn.value).toBe(true)
  })

  it('logout clears the session state', async () => {
    mockFetch('/api/session', { address: ADDR, operator: false })
    mockFetch('/api/auth/logout', { ok: true })
    const { signedIn, logout } = useSession()
    await settle()
    expect(signedIn.value).toBe(true)
    await logout()
    expect(signedIn.value).toBe(false)
  })
})
```

Note: the `beforeEach` mock-reset line is fiddly with top-level `vi.mock`; if TypeScript complains, simplify to just `resetSessionCache()` in `beforeEach` and drop the mockReset (each test sets its own `mockResolvedValue` where needed). Keep it compiling.

**Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/composables/useSession.test.ts`
Expected: FAIL — cannot find module `./useSession`.

**Step 3: Write the composable**

Create `web/src/composables/useSession.ts`:

```ts
import { computed, ref } from 'vue'
import { apiGet, apiPost } from '../api'
import { loginWithHub } from '../hub'

interface SessionInfo {
  signedIn: boolean
  address: string
  operator: boolean
}

const signedOut: SessionInfo = { signedIn: false, address: '', operator: false }
let cache: Promise<SessionInfo> | null = null

function fetchSession(): Promise<SessionInfo> {
  if (!cache) {
    cache = apiGet<{ address: string; operator: boolean }>('/api/session')
      .then((s) => ({ signedIn: true, address: s.address, operator: s.operator }))
      .catch(() => ({ ...signedOut }))
  }
  return cache
}

export function useSession() {
  const state = ref<SessionInfo>({ ...signedOut })
  fetchSession().then((s) => { state.value = s })

  return {
    signedIn: computed(() => state.value.signedIn),
    address: computed(() => state.value.address),
    operator: computed(() => state.value.operator),
    async login() {
      await loginWithHub()
      cache = null
      state.value = await fetchSession()
    },
    async logout() {
      await apiPost('/api/auth/logout')
      cache = null
      state.value = { ...signedOut }
    },
  }
}

export function resetSessionCache() {
  cache = null
}
```

**Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/composables/useSession.test.ts && npm run build`
Expected: PASS + clean build.

**Step 5: Commit**

```bash
git add web/src/composables/useSession.ts web/src/composables/useSession.test.ts
git commit -m "feat: add useSession composable with shared session cache"
```

---

### Task 6: Header session UI

**Files:**
- Modify: `web/src/components/AppHeader.vue`
- Test: `web/src/components/AppHeader.test.ts`

**Step 1: Update the tests (failing first)**

Rewrite `web/src/components/AppHeader.test.ts` (keep the existing theme test, add session cases):

```ts
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AppHeader from './AppHeader.vue'
import { mockFetch } from '../test/helpers'
import { resetSessionCache } from '../composables/useSession'

const ADDR = 'NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE'

function routerForHeader() {
  const placeholder = { template: '<div />' }
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: placeholder },
      { path: '/performance', component: placeholder },
      { path: '/stakers', component: placeholder },
      { path: '/operator', component: placeholder },
      { path: '/me', component: placeholder },
    ],
  })
}

describe('AppHeader', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
    vi.stubGlobal('matchMedia', () => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))
    resetSessionCache()
    mockFetch('/api/session', { error: 'not logged in' }, 401)
  })

  async function mountHeader(session?: { address: string; operator: boolean }) {
    if (session) mockFetch('/api/session', session)
    const router = routerForHeader()
    await router.push('/')
    await router.isReady()
    const wrapper = mount(AppHeader, { global: { plugins: [router] } })
    await new Promise((resolve) => setTimeout(resolve, 0))
    return wrapper
  }

  it('shows a sign-in button when signed out', async () => {
    const wrapper = await mountHeader()
    expect(wrapper.get('.login-btn').text()).toBe('Sign in with Nimiq')
    expect(wrapper.find('.session-chip').exists()).toBe(false)
    expect(wrapper.find('.operator-link').exists()).toBe(true)
  })

  it('shows the session chip and keeps the operator link for operators', async () => {
    const wrapper = await mountHeader({ address: ADDR, operator: true })
    expect(wrapper.get('.session-address').attributes('title')).toBe(ADDR)
    expect(wrapper.get('.session-address').text()).toContain('…')
    expect(wrapper.find('img.identicon').exists()).toBe(true)
    expect(wrapper.find('.operator-link').exists()).toBe(true)
    expect(wrapper.find('.login-btn').exists()).toBe(false)
  })

  it('hides the operator link for signed-in non-operators', async () => {
    const wrapper = await mountHeader({ address: ADDR, operator: false })
    expect(wrapper.find('.operator-link').exists()).toBe(false)
  })

  it('switches the complete application theme and persists the choice', async () => {
    const wrapper = await mountHeader()
    const toggle = wrapper.get('[aria-label="Switch to dark theme"]')
    await toggle.trigger('click')
    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(localStorage.getItem('gopool-theme')).toBe('dark')
    expect(wrapper.get('[aria-label="Switch to light theme"]').element).toBeTruthy()
  })
})
```

**Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/components/AppHeader.test.ts`
Expected: new cases FAIL (no `.login-btn`/`.session-chip`).

**Step 3: Update `AppHeader.vue`**

Script — replace the current imports/setup with:

```ts
import { RouterLink } from 'vue-router'
import { useTheme } from '../composables/useTheme'
import { useSession } from '../composables/useSession'
import Identicon from './ui/Identicon.vue'
import { shortAddress } from '../utils/format'

const { theme, toggleTheme } = useTheme()
const { signedIn, address, operator, login, logout } = useSession()
```

Template — make the Operator link conditional:

```html
<RouterLink v-if="!signedIn || operator" to="/operator" class="operator-link">Operator</RouterLink>
```

Inside `<div class="header-actions">`, after `</nav>` and before the theme toggle button:

```html
<button v-if="!signedIn" type="button" class="login-btn" @click="login">Sign in with Nimiq</button>
<span v-else class="session-chip">
  <Identicon :address="address" :size="22" />
  <RouterLink to="/me" class="session-address" :title="address">{{ shortAddress(address) }}</RouterLink>
  <button type="button" class="signout-btn" @click="logout" aria-label="Sign out">Sign out</button>
</span>
```

Style — add:

```css
.login-btn {
  padding: 10px 16px;
  border: 1px solid rgba(255, 255, 255, .22);
  border-radius: 10px;
  color: white;
  background: rgba(255, 255, 255, .08);
  font-size: .85rem;
  font-weight: 600;
  cursor: pointer;
}
.login-btn:hover { background: rgba(255, 255, 255, .15); }
.session-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 5px 8px 5px 10px;
  border: 1px solid rgba(255, 255, 255, .14);
  border-radius: 999px;
  background: rgba(255, 255, 255, .08);
}
.session-address {
  color: white;
  text-decoration: none;
  font-family: var(--font-mono);
  font-size: .78rem;
  font-weight: 600;
}
.signout-btn {
  border: 0;
  background: none;
  color: rgba(255, 255, 255, .6);
  font-size: .75rem;
  font-weight: 600;
  cursor: pointer;
}
.signout-btn:hover { color: white; }
```

**Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run && npm run build`
Expected: full suite PASS + clean build.

**Step 5: Commit**

```bash
git add web/src/components/AppHeader.vue web/src/components/AppHeader.test.ts
git commit -m "feat: show Nimiq session chip and gated operator link in header"
```

---

### Task 7: Route all login call sites through `useSession`

Without this, logging in from `/me` or the operator layout leaves the header's cached session stale.

**Files:**
- Modify: `web/src/pages/MyDashboard.vue`
- Modify: `web/src/layouts/OperatorLayout.vue`

**Step 1: Update `MyDashboard.vue`**

- Remove `import { loginWithHub } from '../hub'`.
- Add `import { useSession } from '../composables/useSession'` and `const { login: sessionLogin } = useSession()` in `<script setup>`.
- In `login()`, replace `await loginWithHub()` with `await sessionLogin()`.

**Step 2: Update `OperatorLayout.vue`**

- Remove the `loginWithHub` import from `../hub` (line 6).
- Add `import { useSession } from '../composables/useSession'` and `const { login: sessionLogin } = useSession()`.
- In `signIn()`, replace `await loginWithHub()` with `await sessionLogin()` (keep `await verifyAccess()` after it).

Both existing test files (`MyDashboard.test.ts`, `OperatorLayout.test.ts`) mock `../hub` with `vi.mock`; the composable imports the same resolved module, so the mocks keep working without changes. If either test file does not stub global `fetch` yet, add `mockFetch('/api/session', { error: 'not logged in' }, 401)` to its `beforeEach`.

**Step 3: Run tests to verify they pass**

Run: `cd web && npx vitest run && npm run build`
Expected: full suite PASS + clean build.

**Step 4: Commit**

```bash
git add web/src/pages/MyDashboard.vue web/src/layouts/OperatorLayout.vue web/src/pages/MyDashboard.test.ts web/src/layouts/OperatorLayout.test.ts
git commit -m "feat: route all login flows through the shared session composable"
```

(Only stage the test files if they were modified.)

---

### Task 8: StakerLookup auto-loads the signed-in staker's own stake

**Files:**
- Modify: `web/src/pages/StakerLookup.vue`
- Test: `web/src/pages/StakerLookup.test.ts`

**Step 1: Write the failing test**

In `web/src/pages/StakerLookup.test.ts`, add to the `beforeEach` (next to `chartConfigs.length = 0` and `resetExplorerForTests()`):

```ts
mockFetch('/api/session', { error: 'not logged in' }, 401)
```

Add a new test case at the end of the describe:

```ts
it('auto-loads the signed-in staker\'s own position with a Your stake badge', async () => {
  const MINE = 'NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E'
  mockFetch('/api/session', { address: MINE, operator: false })
  mockFetch(`/api/stakers/${encodeURIComponent(MINE)}`, {
    address: MINE, stake_luna: 100_000_000, percentage: 0.4335, payslips: [],
  })
  mockFetch(`/api/stakers/${encodeURIComponent(MINE)}/history`, {
    address: MINE, epochs: [], cumulative_reward_luna: 0,
  })
  const wrapper = await mountPage()
  expect(wrapper.text()).toContain('Your stake')
  expect(wrapper.text()).toContain('1,000 NIM')
})
```

**Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/pages/StakerLookup.test.ts`
Expected: new case FAIL (lookup form shows instead of a position).

**Step 3: Update `StakerLookup.vue`**

Script — add:

```ts
import { useSession } from '../composables/useSession'

const { signedIn, address: sessionAddress } = useSession()
const isOwn = computed(() => sessionAddress.value !== '' && staker.value?.address === sessionAddress.value)
```

Replace the existing route-param watch:

```ts
watch(() => props.address, (a) => { if (a) lookup(a) }, { immediate: true })
```

with:

```ts
watch([() => props.address, () => sessionAddress.value], ([routeAddr, mine]) => {
  if (routeAddr) {
    lookup(routeAddr)
    return
  }
  if (mine) lookup(mine)
}, { immediate: true })
```

Template — in the staker header, add the badge next to the address and make the CTA text session-aware:

```html
<p class="address-line">
  <ExplorerLink kind="account" :value="staker.address" />
  <span v-if="isOwn" class="own-badge">Your stake</span>
</p>
<RouterLink to="/me" class="btn cta-manage">{{ signedIn ? 'Manage your stake' : 'Log in to manage your stake' }}</RouterLink>
```

Style — add:

```css
.own-badge {
  display: inline-block;
  margin-left: 8px;
  padding: 2px 10px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--nimiq-green) 14%, transparent);
  color: var(--nimiq-green);
  font-size: .75rem;
  font-weight: 700;
}
```

**Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run && npm run build`
Expected: full suite PASS + clean build. If the existing "shows a log in to manage CTA" test now fails, it is because the session mock in `beforeEach` makes it signed-out — it should still pass; if the CTA text assertion breaks, confirm the mock returns 401.

**Step 5: Commit**

```bash
git add web/src/pages/StakerLookup.vue web/src/pages/StakerLookup.test.ts
git commit -m "feat: auto-load own stake in Find my stake when signed in"
```

---

### Task 9: Final verification gate

**Step 1: Run every gate**

```bash
go test ./... && go vet ./...
cd web && npm test && npm run build
```

Expected: all PASS, clean build.

**Step 2: Manual smoke test (dev server)**

Start the API + dev web per the repo's usual dev setup (`make` targets or `cmd/api` + `cd web && npm run dev`). Verify:

1. Signed out: header shows "Sign in with Nimiq"; `/stakers` shows the anonymous lookup form with "No wallet connection required".
2. Sign in with a wallet that is in `operator_addresses`: header chip appears with identicon; Operator link stays; operator subnav renders.
3. `/stakers` with no param now shows your own stake with the "Your stake" badge; typing another address still works.
4. Sign out: cookie cleared, header back to sign-in button, Operator link visible again.
5. Identicons visible in: header chip, Find my stake, My dashboard, operator Overview (validator), Operations payout queue, EpochDetail staker table.

**Step 3: Commit any stragglers**

```bash
git status
```

Expected: clean tree (all work committed in Tasks 1-8).
