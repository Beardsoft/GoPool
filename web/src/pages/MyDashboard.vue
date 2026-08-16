<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiGet, apiPut } from '../api'
import { loginWithHub } from '../hub'
import ExplorerLink from '../components/ui/ExplorerLink.vue'
import NimAmount from '../components/ui/NimAmount.vue'
import StatusBadge from '../components/ui/StatusBadge.vue'
import EmptyState from '../components/ui/EmptyState.vue'
import StakerPosition from '../components/StakerPosition.vue'
import { formatNim } from '../utils/format'

interface Payslip {
  batch_number: number
  epoch_number: number
  amount_luna: number
  status: string
  tx_hash?: string
}

interface Tx {
  hash: string
  status: string
  amount_luna: number
  submitted_at: string
}

interface StakerDetail {
  address: string
  stake_luna: number
  percentage: number
  payslips: Payslip[]
  transactions?: Tx[]
  pending_luna: number
  paid_luna: number
  delegated: boolean
  compound: boolean
}

interface StakerHistory {
  cumulative_reward_luna: number
}

const PENDING_STATUSES = ['pending', 'out_for_payment', 'awaiting_confirmation']

const me = ref<StakerDetail | null>(null)
const history = ref<StakerHistory | null>(null)
const loggedIn = ref(false)
const error = ref('')
const compoundError = ref('')
const compoundBusy = ref(false)

async function load() {
  try {
    const [detail, hist] = await Promise.all([
      apiGet<StakerDetail>('/api/me'),
      apiGet<StakerHistory>('/api/me/history')
    ])
    me.value = detail
    history.value = hist
    loggedIn.value = true
  } catch {
    loggedIn.value = false
  }
}

async function login() {
  error.value = ''
  try {
    await loginWithHub()
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}

function downloadPdf() {
  window.print()
}

function isPending(status: string): boolean {
  return PENDING_STATUSES.includes(status)
}

function submittedAtFor(hash?: string): string | undefined {
  if (!hash || !me.value?.transactions) return undefined
  return me.value.transactions.find((t) => t.hash === hash)?.submitted_at
}

function elapsed(ts?: string): string {
  if (!ts) return ''
  const ms = Date.now() - new Date(ts).getTime()
  if (Number.isNaN(ms) || ms < 0) return ''
  const h = Math.floor(ms / 3_600_000)
  const m = Math.floor((ms % 3_600_000) / 60_000)
  return `waiting ${h}h ${m}m`
}

function waitingLabel(status: string, txHash?: string): string {
  if (!isPending(status)) return ''
  return elapsed(submittedAtFor(txHash)) || 'waiting…'
}

async function setCompound(value: boolean) {
  if (!me.value) return
  const prev = me.value.compound
  compoundBusy.value = true
  compoundError.value = ''
  me.value.compound = value
  try {
    await apiPut('/api/me/preference', { compound: value })
  } catch (e) {
    me.value.compound = prev
    compoundError.value = (e as Error).message
  } finally {
    compoundBusy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="card">
    <h1>My dashboard</h1>
    <button v-if="!loggedIn" @click="login" class="btn">Log in with Nimiq Hub</button>
    <p v-if="error" class="error">{{ error }}</p>
    <div v-if="me">
      <p class="muted">Address: <span class="address"><ExplorerLink kind="account" :value="me.address" /></span></p>

      <div class="grid" style="margin-top: var(--space-16);">
        <div class="stat">
          <div class="label">Awaiting payout</div>
          <div class="value">{{ formatNim(me.pending_luna) }} <span class="muted">NIM</span></div>
        </div>
        <div class="stat">
          <div class="label">Paid out</div>
          <div class="value">{{ formatNim(me.paid_luna) }} <span class="muted">NIM</span></div>
        </div>
      </div>

      <div class="profile-row">
        <span class="delegation-badge" :class="me.delegated ? 'ok' : 'off'">
          {{ me.delegated ? 'Delegated to pool' : 'Not delegated' }}
        </span>
        <span v-if="!me.delegated" class="muted hint">Payouts will be sent as transfers, not restaked.</span>
      </div>

      <div class="compound-toggle">
        <label class="compound-label">
          <input type="checkbox" :checked="me.compound" :disabled="compoundBusy" @change="setCompound(($event.target as HTMLInputElement).checked)" />
          <span>{{ me.compound ? 'Reinvest as stake' : 'Pay me out' }}</span>
        </label>
        <p v-if="compoundError" class="error">{{ compoundError }}</p>
      </div>

      <StakerPosition
        :position="{ address: me.address, stake_luna: me.stake_luna, percentage: me.percentage }"
        :history="history"
        export-url="/api/me/payslips.csv"
      />
      <h2 style="margin-top: var(--space-24);">Payslips</h2>
      <div style="display:flex; gap:8px; margin-bottom:12px">
        <button class="btn" @click="downloadPdf">Download PDF</button>
      </div>
      <table v-if="me.payslips.length">
        <thead><tr><th>Batch</th><th>Epoch</th><th>Amount</th><th>Status</th><th>Waiting</th><th>Transaction</th></tr></thead>
        <tbody>
          <tr v-for="p in me.payslips" :key="p.batch_number">
            <td>{{ p.batch_number }}</td>
            <td>{{ p.epoch_number }}</td>
            <td><NimAmount :luna="p.amount_luna" /></td>
            <td><StatusBadge :status="p.status" /></td>
            <td><span v-if="waitingLabel(p.status, p.tx_hash)" class="muted">{{ waitingLabel(p.status, p.tx_hash) }}</span></td>
            <td>
              <ExplorerLink v-if="p.tx_hash" kind="transaction" :value="p.tx_hash" label="View on explorer" />
              <span v-else class="muted">Not sent yet</span>
            </td>
          </tr>
        </tbody>
      </table>
      <EmptyState v-else title="No payouts yet" description="Payouts appear here once your accumulated rewards cross the pool's minimum payout threshold." />
    </div>
  </div>
</template>

<style scoped>
.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px,1fr)); gap: var(--space-16); }
.stat { padding: var(--space-16); border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--bg); }
.label { font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-60); margin-bottom: var(--space-8); }
.value { font-size: 1.25rem; font-weight: 700; }
.profile-row { display: flex; align-items: center; gap: var(--space-12); margin-top: var(--space-16); flex-wrap: wrap; }
.delegation-badge { display: inline-block; padding: 4px 12px; border-radius: 999px; font-size: 0.78rem; font-weight: 700; }
.delegation-badge.ok { background: color-mix(in srgb, var(--nimiq-green) 14%, transparent); color: var(--nimiq-green); }
.delegation-badge.off { background: var(--bg-muted); color: var(--text-60); }
.hint { font-size: 0.82rem; }
.compound-toggle { margin-top: var(--space-16); padding: var(--space-16); border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--bg); }
.compound-label { display: flex; align-items: center; gap: var(--space-8); font-weight: 600; cursor: pointer; }
.compound-label input { width: 18px; height: 18px; accent-color: var(--nimiq-light-blue); }
</style>
