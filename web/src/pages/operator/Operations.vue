<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { apiGet, apiPost } from '../../api'
import HoldConfirmButton from '../../components/ui/HoldConfirmButton.vue'
import NimAmount from '../../components/ui/NimAmount.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import type { OperatorAction, OperatorPayout, PageResponse } from '../../types/api'

const payouts = ref<OperatorPayout[]>([])
const actions = ref<OperatorAction[]>([])
const statusFilter = ref('')
const addressFilter = ref('')
const error = ref('')
const reviewAction = ref<'deactivate' | 'retire' | null>(null)

const filteredPayouts = computed(() => payouts.value
  .filter((item) => !statusFilter.value || item.status === statusFilter.value)
  .filter((item) => !addressFilter.value || item.address.toLowerCase().includes(addressFilter.value.toLowerCase()))
  .sort((a, b) => Number(b.status === 'failed') - Number(a.status === 'failed')))

async function load() {
  try {
    const [payoutPage, actionPage] = await Promise.all([
      apiGet<PageResponse<OperatorPayout>>('/api/operator/payouts?limit=50'),
      apiGet<PageResponse<OperatorAction>>('/api/operator/actions?limit=50'),
    ])
    payouts.value = payoutPage.items
    actions.value = actionPage.items
  } catch (cause: any) {
    error.value = cause.message ?? 'Unable to load operations'
  }
}

async function confirmAction() {
  if (!reviewAction.value) return
  try {
    const queued = await apiPost<OperatorAction>('/api/operator/actions', { action: reviewAction.value })
    actions.value.unshift(queued)
    reviewAction.value = null
  } catch (cause: any) {
    error.value = cause.message ?? 'Unable to queue action'
  }
}

async function retryPayout(hash: string) {
  await apiPost(`/api/operator/payouts/${encodeURIComponent(hash)}/retry`)
  await load()
}

onMounted(load)
</script>

<template>
  <main class="operations-page">
    <header>
      <p class="eyebrow">Operator controls</p>
      <h1>Operations</h1>
      <p class="muted">Automatic payouts remain active. Exceptions requiring attention appear first.</p>
    </header>

    <p v-if="error" class="error">{{ error }}</p>

    <section class="card">
      <div class="section-heading"><h2>Payout queue</h2><span>{{ filteredPayouts.length }} shown</span></div>
      <div class="filters">
        <select v-model="statusFilter" class="input" aria-label="Payout status">
          <option value="">All states</option><option>failed</option><option>pending</option><option>submitted</option><option>confirmed</option>
        </select>
        <input v-model="addressFilter" class="input" placeholder="Filter address" aria-label="Filter payout address" />
      </div>
      <div v-if="filteredPayouts.length" class="table-wrap">
        <table><thead><tr><th>Status</th><th>Recipient</th><th>Amount</th><th>Transaction</th><th></th></tr></thead>
          <tbody><tr v-for="item in filteredPayouts" :key="item.hash">
            <td><StatusBadge :status="item.status || 'unknown'" /></td><td class="address">{{ item.address }}</td>
            <td><NimAmount :luna="item.amount ?? 0" /></td><td class="hash">{{ item.hash }}</td>
            <td><button v-if="item.status === 'failed'" class="btn" @click="retryPayout(item.hash)">Retry</button></td>
          </tr></tbody></table>
      </div>
      <p v-else class="muted">No payout rows match these filters.</p>
    </section>

    <section class="card">
      <h2>Validator action history</h2>
      <ul v-if="actions.length" class="action-list"><li v-for="item in actions" :key="item.id"><StatusBadge :status="item.state" /> {{ item.action }} <span class="muted">{{ item.error_summary }}</span></li></ul>
      <p v-else class="muted">No validator actions have been requested.</p>
    </section>

    <section class="card danger-zone">
      <p class="eyebrow">Danger zone</p><h2>Validator lifecycle</h2>
      <p>These actions are submitted on-chain by the daemon and cannot be treated as active until confirmed.</p>
      <div class="action-buttons"><button class="btn" data-action="deactivate" @click="reviewAction = 'deactivate'">Deactivate</button><button class="btn destructive" data-action="retire" @click="reviewAction = 'retire'">Retire</button></div>
      <div v-if="reviewAction" class="review-panel">
        <h3>Review {{ reviewAction }}</h3><p>This queues a {{ reviewAction }} request. The daemon will validate, sign, and reconcile its chain result.</p>
        <HoldConfirmButton @confirm="confirmAction" />
      </div>
    </section>
  </main>
</template>

<style scoped>
.operations-page{display:grid;gap:24px}.eyebrow{text-transform:uppercase;letter-spacing:.08em;font-size:.75rem;font-weight:700;color:var(--nimiq-light-blue)}
.section-heading,.filters,.action-buttons{display:flex;gap:12px;align-items:center;justify-content:space-between}.filters{margin-bottom:16px}.filters>*{max-width:280px}
.table-wrap{overflow:auto}.address,.hash{max-width:240px;overflow:hidden;text-overflow:ellipsis}.action-list{display:grid;gap:8px;list-style:none;padding:0}.danger-zone{border-color:var(--nimiq-red)}
.destructive{background:var(--nimiq-red)}.review-panel{margin-top:16px;padding:16px;background:var(--bg-muted);border-radius:10px}@media(max-width:640px){.filters,.action-buttons{align-items:stretch;flex-direction:column}.filters>*{max-width:none}}
</style>
