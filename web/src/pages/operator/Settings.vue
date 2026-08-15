<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { apiGet, apiPut } from '../../api'
import HoldConfirmButton from '../../components/ui/HoldConfirmButton.vue'
import type { EditableConfig, SettingsResponse } from '../../types/api'

const response = ref<SettingsResponse | null>(null)
const settings = ref<EditableConfig | null>(null)
const original = ref<EditableConfig | null>(null)
const review = ref(false)
const error = ref('')
const savedHash = ref('')
const minPayoutNim = computed({ get: () => (settings.value?.min_payout_luna ?? 0) / 100_000, set: value => { if (settings.value) settings.value.min_payout_luna = Math.round(Number(value) * 100_000) } })

async function load() {
  try {
    response.value = await apiGet<SettingsResponse>('/api/operator/settings')
    settings.value = JSON.parse(JSON.stringify(response.value.settings))
    original.value = JSON.parse(JSON.stringify(response.value.settings))
  } catch (cause: any) { error.value = cause.message ?? 'Unable to load settings' }
}

async function save() {
  if (!response.value || !settings.value) return
  try {
    const revision = await apiPut<{ hash: string }>('/api/operator/settings', { expected_hash: response.value.active_hash, settings: settings.value })
    savedHash.value = revision.hash; review.value = false; response.value.restart_required = true
  } catch (cause: any) { error.value = cause.message ?? 'Unable to save settings' }
}

onMounted(() => queueMicrotask(load))
</script>

<template>
  <main class="settings-page">
    <header><p class="eyebrow">Revisioned configuration</p><h1>Settings</h1><p class="muted">Changes are written atomically and remain pending until a matching daemon heartbeat.</p></header>
    <p v-if="error" class="error">{{ error }}</p>
    <template v-if="settings && original">
      <p v-if="response?.restart_required || savedHash" class="notice">Restart required — written configuration is not active yet.</p>
      <section class="card form-grid"><h2>Economics</h2><label>Pool fee (%)<input v-model.number="settings.pool_fee_percentage" class="input" type="number" step="0.01" /></label><label>Minimum payout (NIM)<input v-model="minPayoutNim" name="min_payout_nim" class="input" type="number" step="0.00001" /></label><label>Payout mode<select v-model="settings.payout_mode" class="input"><option value="delegate">Delegate</option><option value="transfer">Transfer</option></select></label><label><input v-model="settings.auto_reactivate" type="checkbox" /> Auto-reactivate</label></section>
      <section class="card form-grid"><h2>Public profile</h2><label>Pool name<input v-model="settings.pool_name" class="input" /></label><label>Description<textarea v-model="settings.pool_description" class="input" /></label><label>Contact URL<input v-model="settings.contact_url" class="input" /></label></section>
      <section class="card form-grid"><h2>Access and alerts</h2><label>Operator addresses<input v-model="settings.operator_addresses" class="input address" /></label><label><input v-model="settings.alert_telegram_enabled" type="checkbox" /> Telegram</label><label><input v-model="settings.alert_webhook_enabled" type="checkbox" /> Webhook</label><label><input v-model="settings.alert_email_enabled" type="checkbox" /> Email</label><div class="secret-states"><span v-for="(state, name) in response?.secrets" :key="name">{{ name }}: {{ state }}</span></div></section>
      <button class="btn" data-review @click="review = true">Review changes</button>
      <section v-if="review" class="card review-panel" data-review-panel><h2>Review before saving</h2><p>Minimum payout: {{ original.min_payout_luna / 100_000 }} NIM → {{ settings.min_payout_luna / 100_000 }} NIM</p><p>Pool fee: {{ (original.pool_fee_percentage * 100).toFixed(2) }}% → {{ (settings.pool_fee_percentage * 100).toFixed(2) }}%</p><HoldConfirmButton @confirm="save" /></section>
    </template>
    <p v-else-if="!error">Loading settings…</p>
  </main>
</template>

<style scoped>
.settings-page{display:grid;gap:24px}.eyebrow{text-transform:uppercase;letter-spacing:.08em;font-size:.75rem;font-weight:700;color:var(--nimiq-light-blue)}.form-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.form-grid h2{grid-column:1/-1}.form-grid label{display:grid;gap:6px;font-weight:600}.secret-states{grid-column:1/-1;display:flex;gap:16px;flex-wrap:wrap}.notice{padding:12px 16px;border-radius:10px;background:color-mix(in srgb,var(--nimiq-gold) 18%,var(--bg-elev))}.review-panel{border-color:var(--nimiq-gold)}@media(max-width:640px){.form-grid{grid-template-columns:1fr}}
</style>
