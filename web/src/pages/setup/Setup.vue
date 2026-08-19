<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { apiGet, apiPost } from '../../api'
import type { SetupDraft } from '../../types/api'
import SystemCheck from './steps/SystemCheck.vue'
import ValidatorIdentity from './steps/ValidatorIdentity.vue'
import PoolEconomics from './steps/PoolEconomics.vue'
import PublicProfile from './steps/PublicProfile.vue'
import AccessAlerts from './steps/AccessAlerts.vue'
import ReviewLaunch from './steps/ReviewLaunch.vue'

const steps = ['system', 'validator', 'economics', 'profile', 'access', 'review'] as const
const current = ref(0)
const token = ref('')
const checks = ref<Record<string, unknown>>({})
const fieldErrors = ref<Record<string, string>>({})
const busy = ref(false)
const launched = ref(false)
const live = ref(false)
const readinessError = ref('')
const revisionHash = ref('')
const draft = ref<SetupDraft>({ rpc_url: 'https://rpc-mainnet.nimiqscan.com', network: 'main-albatross', pool_fee_wallet: '', pool_fee_percentage: .01, payout_mode: 'delegate', min_payout_luna: 1_000_000, auto_reactivate: true, api_addr: ':8080', validator_address: '', operator_addresses: '', metrics_addr: ':9100', alert_telegram_enabled: false, alert_webhook_enabled: false, pool_name: 'GoPool', alert_telegram_token: '', alert_webhook_url: '' })
const step = computed(() => steps[current.value])
const component = computed(() => [SystemCheck, ValidatorIdentity, PoolEconomics, PublicProfile, AccessAlerts, ReviewLaunch][current.value])
const progressScale = computed(() => (current.value + 1) / steps.length)
const route = useRoute()
const router = useRouter()

let pollTimer: ReturnType<typeof setTimeout> | undefined
let pollStopped = false
onUnmounted(() => {
  pollStopped = true
  clearTimeout(pollTimer)
})

type SetupStatus = {
  checks: Record<string, unknown>
  hints?: Partial<Pick<SetupDraft, 'validator_address' | 'pool_fee_wallet' | 'network' | 'rpc_url'>>
}

function applyHints(status: SetupStatus) {
  const hints = status.hints
  if (!hints) return
  if (hints.validator_address) {
    draft.value.validator_address = hints.validator_address
    if (!draft.value.pool_fee_wallet) draft.value.pool_fee_wallet = hints.validator_address
  }
  if (hints.pool_fee_wallet) draft.value.pool_fee_wallet = hints.pool_fee_wallet
  if (hints.network) draft.value.network = hints.network
  if (hints.rpc_url) draft.value.rpc_url = hints.rpc_url
}

async function pollActivation() {
  if (pollStopped) return
  try {
    const readiness = await apiGet<{ readiness_error?: string | null }>('/api/operator/readiness')
    if (readiness.readiness_error) {
      readinessError.value = readiness.readiness_error
      return
    }
  } catch {
    // 401 until an operator session exists; keep polling /api/pool.
  }
  try {
    await apiGet('/api/pool')
    live.value = true
    return
  } catch {
    // still activating
  }
  pollTimer = setTimeout(pollActivation, 1000)
}

async function next() {
  busy.value = true; fieldErrors.value = {}
  try {
    if (step.value === 'system') {
      await apiPost('/api/setup/session', { token: token.value })
      const status = await apiGet<SetupStatus>('/api/setup/status')
      checks.value = status.checks
      applyHints(status)
      current.value++
      return
    }
    if (step.value === 'review') {
      const revision = await apiPost<{ hash: string }>('/api/setup/complete', { settings: draft.value })
      revisionHash.value = revision.hash; launched.value = true
      void pollActivation()
      return
    }
    const result = await apiPost<{ valid: boolean; field_errors: Record<string, string> }>('/api/setup/validate', draft.value)
    fieldErrors.value = result.field_errors ?? {}
    if (Object.keys(fieldErrors.value).length === 0 && result.valid !== false) current.value++
  } catch (cause: any) { fieldErrors.value = { _form: cause.message ?? 'Setup request failed' } }
  finally { busy.value = false }
}

onMounted(() => {
  const queryToken = route.query.token
  if (typeof queryToken !== 'string' || queryToken === '') return
  token.value = queryToken
  void router.replace({ path: '/setup', query: {} })
  void next()
})
</script>

<template>
  <main class="setup-assistant" :data-step="step">
    <header><p class="eyebrow">First-run setup</p><h1>Launch your GoPool</h1><p>Step {{ current + 1 }} of {{ steps.length }}</p><div class="progress"><span :style="{ transform: `scaleX(${progressScale})` }" /></div></header>
    <section v-if="launched" class="card launch-state">
      <h2 v-if="live">Pool is live</h2>
      <h2 v-else>Activating configuration…</h2>
      <p>Revision <code>{{ revisionHash }}</code></p>
      <p v-if="readinessError" class="error">{{ readinessError }}</p>
      <p v-else-if="!live">Waiting for the daemon heartbeat and validator readiness.</p>
    </section>
    <section v-else class="card step-card">
      <component :is="component" v-model:draft="draft" :token="token" :checks="checks" @update:token="token = $event" />
      <ul v-if="Object.keys(fieldErrors).length" class="error-list"><li v-for="(message, name) in fieldErrors" :key="name">{{ message }}</li></ul>
      <footer><button v-if="current > 0" class="btn secondary" @click="current--">Back</button><button class="btn" data-next :disabled="busy" @click="next">{{ step === 'review' ? 'Write configuration' : 'Continue' }}</button></footer>
    </section>
  </main>
</template>

<style scoped>
.setup-assistant{max-width:720px;margin:0 auto;display:grid;gap:24px}.eyebrow{text-transform:uppercase;letter-spacing:.08em;font-size:.75rem;font-weight:700;color:var(--nimiq-light-blue)}.progress{height:6px;background:var(--bg-muted);border-radius:999px;overflow:hidden}.progress span{display:block;height:100%;width:100%;transform:scaleX(0);transform-origin:left center;background:var(--nimiq-green);transition:transform .2s}.step-card :deep(section){display:grid;gap:16px}.step-card :deep(label){display:grid;gap:6px;font-weight:600}.step-card footer{display:flex;justify-content:flex-end;gap:12px;margin-top:24px}.secondary{background:var(--bg-muted);color:var(--text-100)}.error-list,.error{color:var(--nimiq-red)}
</style>
