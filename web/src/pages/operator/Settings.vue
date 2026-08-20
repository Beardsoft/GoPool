<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { apiGet, apiPut } from '../../api'
import HoldConfirmButton from '../../components/ui/HoldConfirmButton.vue'
import type { AlertSecrets, EditableConfig, SettingsResponse } from '../../types/api'

const emptySecrets = (): AlertSecrets => ({ alert_telegram_token: '', alert_webhook_url: '' })

const response = ref<SettingsResponse | null>(null)
const settings = ref<EditableConfig | null>(null)
const secrets = ref<AlertSecrets>(emptySecrets())
const original = ref<EditableConfig | null>(null)
const review = ref(false)
const error = ref('')
const savedHash = ref('')
let pollTimer: ReturnType<typeof setTimeout> | undefined
let pollStopped = false
onUnmounted(() => {
  pollStopped = true
  clearTimeout(pollTimer)
})
const minPayoutNim = computed({
  get: () => (settings.value?.min_payout_luna ?? 0) / 100_000,
  set: (value) => {
    if (settings.value) settings.value.min_payout_luna = Math.round(Number(value) * 100_000)
  },
})

async function load() {
  try {
    response.value = await apiGet<SettingsResponse>('/api/operator/settings')
    settings.value = JSON.parse(JSON.stringify(response.value.settings))
    original.value = JSON.parse(JSON.stringify(response.value.settings))
  } catch (cause: any) {
    error.value = cause.message ?? 'Unable to load settings'
  }
}

async function pollUntilActive() {
  if (pollStopped) return
  await load()
  if (response.value?.restart_required) {
    pollTimer = setTimeout(pollUntilActive, 1000)
  }
}

const changedSecrets = computed(() => Object.entries(secrets.value).filter(([, value]) => value !== '').map(([name]) => name))

async function save() {
  if (!response.value || !settings.value) return
  try {
    const revision = await apiPut<{ hash: string }>('/api/operator/settings', {
      expected_hash: response.value.active_hash,
      settings: settings.value,
      secrets: secrets.value,
    })
    savedHash.value = revision.hash
    review.value = false
    response.value.restart_required = true
    void pollUntilActive()
  } catch (cause: any) {
    error.value = cause.message ?? 'Unable to save settings'
  }
}

onMounted(() => queueMicrotask(async () => {
  await load()
  if (response.value?.restart_required) void pollUntilActive()
}))
</script>

<template>
  <main class="settings-page">
    <header>
      <p class="eyebrow">Revisioned configuration</p>
      <h1>Settings</h1>
      <p class="muted">Changes are written atomically and remain pending until a matching daemon heartbeat.</p>
    </header>

    <p v-if="error" class="error">{{ error }}</p>

    <template v-if="settings && original">
      <p v-if="response?.restart_required || savedHash" class="notice">
        {{ response?.restart_required ? 'Activating… written configuration is not active yet.' : 'Configuration is active.' }}
      </p>

      <section class="card form-grid">
        <h2>Economics</h2>
        <label>
          Pool fee (%)
          <input v-model.number="settings.pool_fee_percentage" class="input" type="number" step="0.01" />
        </label>
        <label>
          Minimum payout (NIM)
          <input v-model="minPayoutNim" name="min_payout_nim" class="input" type="number" step="0.00001" />
        </label>
        <label>
          Payout mode
          <select v-model="settings.payout_mode" class="input">
            <option value="delegate">Delegate</option>
            <option value="transfer">Transfer</option>
          </select>
        </label>
        <label class="check-field">
          <input v-model="settings.auto_reactivate" type="checkbox" />
          <span>Auto-reactivate</span>
        </label>
      </section>

      <section class="card form-grid">
        <h2>Public profile</h2>
        <label class="span-2">
          Pool name
          <input v-model="settings.pool_name" class="input" />
        </label>
        <label class="span-2">
          Description
          <textarea v-model="settings.pool_description" class="input" rows="3" />
        </label>
        <label class="span-2">
          Contact URL
          <input v-model="settings.contact_url" class="input" type="url" />
        </label>
        <label>
          Telegram URL
          <input v-model="settings.telegram_url" name="telegram_url" class="input" type="url" placeholder="https://t.me/yourpool" />
        </label>
        <label>
          Discord URL
          <input v-model="settings.discord_url" name="discord_url" class="input" type="url" placeholder="https://discord.gg/invite" />
        </label>
        <label class="span-2">
          X URL
          <input v-model="settings.x_url" name="x_url" class="input" type="url" placeholder="https://x.com/yourpool" />
        </label>
        <p class="muted span-2">Public links on the pool website. Separate from Telegram/Discord alert channels below.</p>
        <label class="span-2">
          Disclosure
          <textarea v-model="settings.disclosure" name="disclosure" class="input" rows="3" />
        </label>
      </section>

      <section class="card form-grid">
        <h2>Access and alerts</h2>
        <label class="span-2">
          Operator addresses
          <input
            v-model="settings.operator_addresses"
            class="input address"
            placeholder="Comma-separated Nimiq addresses"
          />
        </label>

        <fieldset class="alert-channels span-2">
          <legend>Alert channels</legend>

          <h3>Telegram</h3>
          <label class="check-field">
            <input v-model="settings.alert_telegram_enabled" type="checkbox" />
            <span>Telegram</span>
          </label>
          <label>
            Telegram destination (chat ID)
            <input v-model="settings.alert_telegram_destination" class="input" placeholder="e.g. 123456789" />
          </label>
          <label>
            Telegram bot token
            <input v-model="secrets.alert_telegram_token" class="input" type="password" placeholder="••• configured — leave blank to keep" autocomplete="new-password" />
          </label>
          <h3>Webhook</h3>
          <label class="check-field">
            <input v-model="settings.alert_webhook_enabled" type="checkbox" />
            <span>Webhook</span>
          </label>
          <label>
            Webhook URL (Discord webhook URLs work)
            <input v-model="secrets.alert_webhook_url" class="input" type="password" placeholder="••• configured — leave blank to keep" autocomplete="new-password" />
          </label>
          <p class="secret-hint">Leave a secret blank to keep its current value. Secret values are never shown after saving.</p>
        </fieldset>

        <div v-if="response?.secrets" class="secret-states span-2">
          <span
            v-for="(state, name) in response.secrets"
            :key="name"
            class="secret-chip"
            :data-state="state"
          >
            <strong>{{ name }}</strong>
            {{ state }}
          </span>
        </div>
      </section>

      <button class="btn" data-review @click="review = true">Review changes</button>

      <section v-if="review" class="card review-panel" data-review-panel>
        <h2>Review before saving</h2>
        <p>Minimum payout: {{ original.min_payout_luna / 100_000 }} NIM → {{ settings.min_payout_luna / 100_000 }} NIM</p>
        <p>Pool fee: {{ (original.pool_fee_percentage * 100).toFixed(2) }}% → {{ (settings.pool_fee_percentage * 100).toFixed(2) }}%</p>
        <p v-if="changedSecrets.length">Secrets updated: {{ changedSecrets.join(', ') }}. Values are stored and never shown again.</p>
        <HoldConfirmButton @confirm="save" />
      </section>
    </template>

    <p v-else-if="!error" class="muted">Loading settings…</p>
  </main>
</template>

<style scoped>
.settings-page {
  display: grid;
  gap: 24px;
}
.eyebrow {
  margin-bottom: 8px;
  text-transform: uppercase;
  letter-spacing: .08em;
  font-size: .75rem;
  font-weight: 700;
  color: var(--nimiq-light-blue);
}
.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 18px 16px;
  align-items: start;
}
.form-grid h2 {
  grid-column: 1 / -1;
  margin: 0 0 4px;
}
.form-grid label {
  display: grid;
  gap: 6px;
  font-weight: 600;
}
.span-2 {
  grid-column: 1 / -1;
}
.check-field {
  display: flex !important;
  align-items: center;
  gap: 10px;
  min-height: 44px;
  font-weight: 600;
}
.check-field input[type="checkbox"] {
  width: 18px;
  height: 18px;
  margin: 0;
  accent-color: var(--nimiq-light-blue);
  flex-shrink: 0;
}
.alert-channels {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px 16px;
  margin: 0;
  padding: 14px 16px;
  border: 1px solid var(--app-border);
  border-radius: 10px;
  background: var(--surface-2);
}
.alert-channels legend {
  grid-column: 1 / -1;
}
.alert-channels .secret-hint {
  grid-column: 1 / -1;
  margin: 0;
  color: var(--app-muted);
  font-size: .8rem;
  font-weight: 500;
}
.alert-channels legend {
  padding: 0 6px;
  color: var(--app-muted);
  font-size: .78rem;
  font-weight: 700;
  letter-spacing: .04em;
  text-transform: uppercase;
}
.alert-channels h3 {
  grid-column: 1 / -1;
  margin: 6px 0 0;
  color: var(--app-muted);
  font-size: .78rem;
  font-weight: 700;
  letter-spacing: .04em;
  text-transform: uppercase;
}
.secret-states {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.secret-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 7px 12px;
  border-radius: 999px;
  background: var(--surface-2);
  color: var(--app-muted);
  font-size: .8rem;
  font-weight: 600;
}
.secret-chip strong {
  color: var(--app-text);
  font-weight: 700;
}
.secret-chip[data-state="configured"] {
  color: var(--nimiq-green, #21b66f);
  background: color-mix(in srgb, var(--nimiq-green, #21b66f) 14%, var(--surface-2));
}
.secret-chip[data-state="missing"] {
  color: var(--danger-text);
  background: var(--danger-soft);
}
.notice {
  padding: 12px 16px;
  border-radius: 10px;
  background: color-mix(in srgb, var(--nimiq-gold) 18%, var(--bg-elev));
}
.review-panel {
  border-color: var(--nimiq-gold);
}
@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
  .alert-channels {
    grid-template-columns: 1fr;
  }
}
</style>
