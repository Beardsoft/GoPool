<script setup lang="ts">
import { computed, ref } from 'vue'
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
const revisionHash = ref('')
const draft = ref<SetupDraft>({ rpc_url: 'https://rpc-testnet.nimiqscan.com', network: 'test-albatross', pool_fee_wallet: '', pool_fee_percentage: .01, payout_mode: 'delegate', min_payout_luna: 1_000_000, auto_reactivate: true, api_addr: ':8080', validator_address: '', operator_addresses: '', metrics_addr: ':9100', alert_telegram_enabled: false, alert_webhook_enabled: false, alert_email_enabled: false, pool_name: 'GoPool', alert_telegram_token: '', alert_email_smtp_host: '', alert_email_smtp_port: 0, alert_email_username: '', alert_email_password: '', alert_email_from: '' })
const step = computed(() => steps[current.value])
const component = computed(() => [SystemCheck, ValidatorIdentity, PoolEconomics, PublicProfile, AccessAlerts, ReviewLaunch][current.value])

async function next() {
  busy.value = true; fieldErrors.value = {}
  try {
    if (step.value === 'system') {
      await apiPost('/api/setup/session', { token: token.value })
      const status = await apiGet<{ checks: Record<string, unknown> }>('/api/setup/status')
      checks.value = status.checks
      current.value++
      return
    }
    if (step.value === 'review') {
      const revision = await apiPost<{ hash: string }>('/api/setup/complete', { settings: draft.value })
      revisionHash.value = revision.hash; launched.value = true; return
    }
    const result = await apiPost<{ valid: boolean; field_errors: Record<string, string> }>('/api/setup/validate', draft.value)
    fieldErrors.value = result.field_errors ?? {}
    if (Object.keys(fieldErrors.value).length === 0 && result.valid !== false) current.value++
  } catch (cause: any) { fieldErrors.value = { _form: cause.message ?? 'Setup request failed' } }
  finally { busy.value = false }
}
</script>

<template>
  <main class="setup-assistant" :data-step="step">
    <header><p class="eyebrow">First-run setup</p><h1>Launch your GoPool</h1><p>Step {{ current + 1 }} of {{ steps.length }}</p><div class="progress"><span :style="{ width: `${((current + 1) / steps.length) * 100}%` }" /></div></header>
    <section v-if="launched" class="card launch-state"><h2>Configuration written</h2><p>Revision <code>{{ revisionHash }}</code> is pending activation.</p><pre>docker compose -f deployments/docker-compose.yml restart gopool gopool-api</pre><p>Readiness remains pending until the daemon heartbeat reports this revision and verifies the validator address.</p></section>
    <section v-else class="card step-card">
      <component :is="component" v-model:draft="draft" :token="token" :checks="checks" @update:token="token = $event" />
      <ul v-if="Object.keys(fieldErrors).length" class="error-list"><li v-for="(message, name) in fieldErrors" :key="name">{{ message }}</li></ul>
      <footer><button v-if="current > 0" class="btn secondary" @click="current--">Back</button><button class="btn" data-next :disabled="busy" @click="next">{{ step === 'review' ? 'Write configuration' : 'Continue' }}</button></footer>
    </section>
  </main>
</template>

<style scoped>
.setup-assistant{max-width:720px;margin:0 auto;display:grid;gap:24px}.eyebrow{text-transform:uppercase;letter-spacing:.08em;font-size:.75rem;font-weight:700;color:var(--nimiq-light-blue)}.progress{height:6px;background:var(--bg-muted);border-radius:999px;overflow:hidden}.progress span{display:block;height:100%;background:var(--nimiq-green);transition:width .2s}.step-card :deep(section){display:grid;gap:16px}.step-card :deep(label){display:grid;gap:6px;font-weight:600}.step-card footer{display:flex;justify-content:flex-end;gap:12px;margin-top:24px}.secondary{background:var(--bg-muted);color:var(--text-100)}.error-list{color:var(--nimiq-red)}pre{overflow:auto;padding:12px;background:var(--bg-muted);border-radius:10px}
</style>
