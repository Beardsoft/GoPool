<script setup lang="ts">
import { onMounted, ref } from 'vue'
import AppHeader from '../components/AppHeader.vue'
import OperatorNav from '../components/OperatorNav.vue'
import { apiGet, type ApiError } from '../api'
import { loginWithHub } from '../hub'
import type { OperatorOverview } from '../types/api'

type AccessState = 'checking' | 'signed-out' | 'denied' | 'ready' | 'error'

const access = ref<AccessState>('checking')
const message = ref('')
const signingIn = ref(false)

async function verifyAccess() {
  access.value = 'checking'
  message.value = ''
  try {
    await apiGet<OperatorOverview>('/api/operator/overview')
    access.value = 'ready'
  } catch (cause) {
    const error = cause as ApiError
    if (error.status === 401) access.value = 'signed-out'
    else if (error.status === 403) access.value = 'denied'
    else {
      access.value = 'error'
      message.value = error.message || 'The operator API could not be reached.'
    }
  }
}

async function signIn() {
  signingIn.value = true
  message.value = ''
  try {
    await loginWithHub()
    await verifyAccess()
  } catch (cause) {
    const error = cause as ApiError
    message.value = error.message || 'Nimiq Hub sign-in did not complete.'
    access.value = error.status === 403 ? 'denied' : 'signed-out'
  } finally {
    signingIn.value = false
  }
}

onMounted(verifyAccess)
</script>

<template>
  <div class="operator-layout">
    <AppHeader />
    <main v-if="access === 'ready'" class="operator-container">
      <OperatorNav />
      <RouterView />
    </main>

    <main v-else class="access-stage">
      <section v-if="access === 'checking'" class="access-card" aria-live="polite">
        <div class="access-symbol checking-symbol" aria-hidden="true"></div>
        <p class="access-kicker">Operator console</p>
        <h1>Checking your session</h1>
        <p class="access-copy">Confirming secure access to this pool.</p>
      </section>

      <section v-else class="access-card" data-operator-gateway>
        <div class="access-symbol" aria-hidden="true">
          <svg viewBox="0 0 24 24"><path d="M7 10V8a5 5 0 0 1 10 0v2M6 10h12v10H6z"/><path d="M12 14v2"/></svg>
        </div>
        <p class="access-kicker">Private operator workspace</p>
        <h1>{{ access === 'denied' ? 'This address is not an operator' : access === 'error' ? 'Console unavailable' : 'Operate your pool with confidence' }}</h1>
        <p class="access-copy">
          {{ access === 'denied'
            ? 'Sign in with an address configured for this pool, or return to the public pool.'
            : access === 'error'
              ? 'The API did not respond. Check the service, then try again.'
              : 'Review health, investigate payouts, manage alerts, and make protected changes after signing in with Nimiq Hub.' }}
        </p>
        <p v-if="message" class="access-error" role="alert">{{ message }}</p>
        <div class="access-actions">
          <button v-if="access !== 'error'" type="button" class="primary-action" :disabled="signingIn" @click="signIn">
            {{ signingIn ? 'Opening Nimiq Hub…' : 'Sign in with Nimiq Hub' }}
          </button>
          <button v-else type="button" class="primary-action" @click="verifyAccess">Try again</button>
          <RouterLink to="/" class="secondary-action">Back to public pool</RouterLink>
        </div>
        <div class="access-assurances" aria-label="Security assurances">
          <span>Hub-authenticated</span>
          <span>Session protected</span>
          <span>Signer isolated</span>
        </div>
      </section>
    </main>
  </div>
</template>

<style scoped>
.operator-layout {
  min-height: 100vh;
}
.operator-container {
  max-width: 1120px;
  margin: 0 auto;
  padding: 0 24px 64px;
}
.access-stage {
  min-height: calc(100vh - 77px);
  display: grid;
  place-items: center;
  padding: 48px 24px;
  background:
    radial-gradient(circle at 15% 20%, rgba(5,130,202,.12), transparent 32%),
    radial-gradient(circle at 85% 75%, rgba(233,178,19,.1), transparent 28%);
}
.access-card {
  width: min(100%, 620px);
  padding: 56px;
  text-align: center;
  border-radius: 18px;
  background: var(--surface-1);
  box-shadow: var(--shadow-elevated);
}
.access-symbol {
  width: 72px;
  height: 78px;
  display: grid;
  place-items: center;
  margin: 0 auto 28px;
  color: white;
  background: var(--nimiq-light-blue-bg);
  clip-path: polygon(25% 6%, 75% 6%, 100% 50%, 75% 94%, 25% 94%, 0 50%);
}
.access-symbol svg { width: 32px; height: 32px; fill: none; stroke: currentColor; stroke-width: 1.7; stroke-linecap: round; stroke-linejoin: round; }
.checking-symbol { animation: breathe 1.6s var(--nimiq-ease) infinite alternate; }
.access-kicker { margin-bottom: 12px; color: var(--nimiq-light-blue); font-weight: 700; }
.access-card h1 { max-width: 520px; margin: 0 auto 18px; font-size: clamp(2rem, 5vw, 3.1rem); letter-spacing: -.035em; }
.access-copy { max-width: 52ch; margin: 0 auto 28px; color: var(--app-muted); font-size: 1.05rem; }
.access-error { margin: 0 auto 20px; padding: 12px 16px; border-radius: 10px; color: var(--danger-text); background: var(--danger-soft); }
.access-actions { display: flex; justify-content: center; align-items: center; gap: 12px; flex-wrap: wrap; }
.primary-action, .secondary-action { min-height: 46px; padding: 0 22px; border-radius: 999px; font: inherit; font-weight: 700; cursor: pointer; }
.primary-action { border: 0; color: white; background: var(--nimiq-light-blue-bg); box-shadow: 0 10px 24px rgba(5,130,202,.24); }
.primary-action:disabled { opacity: .6; cursor: wait; }
.secondary-action { display: inline-flex; align-items: center; color: var(--app-text); border: 1px solid var(--app-border); background: transparent; text-decoration: none; }
.access-assurances { display: flex; justify-content: center; gap: 18px; margin-top: 32px; color: var(--app-faint); font-size: .78rem; font-weight: 700; }
.access-assurances span::before { content: '✓'; margin-right: 6px; color: var(--nimiq-green); }
@keyframes breathe { to { transform: scale(.94); opacity: .7; } }

@media (max-width: 640px) {
  .operator-container { padding: 0 16px 48px; }
  .access-stage { padding: 32px 16px; }
  .access-card { padding: 40px 24px; }
  .access-assurances { flex-direction: column; gap: 8px; }
}
</style>
