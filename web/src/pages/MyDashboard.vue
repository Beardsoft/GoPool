<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiGet } from '../api'
import { loginWithHub } from '../hub'
import ExplorerLink from '../components/ui/ExplorerLink.vue'
import NimAmount from '../components/ui/NimAmount.vue'
import StatusBadge from '../components/ui/StatusBadge.vue'
import EmptyState from '../components/ui/EmptyState.vue'
import StakerPosition from '../components/StakerPosition.vue'

interface Payslip {
  batch_number: number
  epoch_number: number
  amount_luna: number
  status: string
  tx_hash?: string
}

interface StakerDetail {
  address: string
  stake_luna: number
  percentage: number
  payslips: Payslip[]
}

interface StakerHistory {
  cumulative_reward_luna: number
}

const me = ref<StakerDetail | null>(null)
const history = ref<StakerHistory | null>(null)
const loggedIn = ref(false)
const error = ref('')

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

onMounted(load)
</script>

<template>
  <div class="card">
    <h1>My dashboard</h1>
    <button v-if="!loggedIn" @click="login" class="btn">Log in with Nimiq Hub</button>
    <p v-if="error" class="error">{{ error }}</p>
    <div v-if="me">
      <p class="muted">Address: <span class="address"><ExplorerLink kind="account" :value="me.address" /></span></p>
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
        <thead><tr><th>Batch</th><th>Epoch</th><th>Amount</th><th>Status</th><th>Transaction</th></tr></thead>
        <tbody>
          <tr v-for="p in me.payslips" :key="p.batch_number">
            <td>{{ p.batch_number }}</td>
            <td>{{ p.epoch_number }}</td>
            <td><NimAmount :luna="p.amount_luna" /></td>
            <td><StatusBadge :status="p.status" /></td>
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
</style>
