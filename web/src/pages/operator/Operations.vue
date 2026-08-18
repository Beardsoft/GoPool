<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { apiGet, apiPost } from '../../api'
import ExplorerLink from '../../components/ui/ExplorerLink.vue'
import HoldConfirmButton from '../../components/ui/HoldConfirmButton.vue'
import NimAmount from '../../components/ui/NimAmount.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import type { OperatorAction, OperatorPayout, PageResponse } from '../../types/api'

const payouts = ref<OperatorPayout[]>([])
const actions = ref<OperatorAction[]>([])
const statusFilter = ref('')
const addressFilter = ref('')
const epochFilter = ref<number | null>(null)
const error = ref('')
const reviewAction = ref<'deactivate' | 'retire' | null>(null)
const payoutNextCursor = ref<number | string | null>(null)
const payoutHasMore = ref(false)
const loadingMore = ref(false)

const filteredPayouts = computed(() => payouts.value
  .filter((item) => !statusFilter.value || item.status === statusFilter.value)
  .filter((item) => !addressFilter.value || item.address.toLowerCase().includes(addressFilter.value.toLowerCase()))
  .filter((item) => epochFilter.value == null || (item.epoch_from != null && item.epoch_to != null && epochFilter.value >= item.epoch_from && epochFilter.value <= item.epoch_to))
  .sort((a, b) => Number(b.status === 'failed') - Number(a.status === 'failed')))

async function load() {
  try {
    const [payoutPage, actionPage] = await Promise.all([
      apiGet<PageResponse<OperatorPayout>>('/api/operator/payouts?limit=50'),
      apiGet<PageResponse<OperatorAction>>('/api/operator/actions?limit=50'),
    ])
    payouts.value = payoutPage.items
    payoutNextCursor.value = payoutPage.next_cursor
    payoutHasMore.value = payoutPage.has_more ?? false
    actions.value = actionPage.items
  } catch (cause: any) {
    error.value = cause.message ?? 'Unable to load operations'
  }
}

async function loadMorePayouts() {
  if (payoutNextCursor.value == null || loadingMore.value) return
  loadingMore.value = true
  try {
    const page = await apiGet<PageResponse<OperatorPayout>>(`/api/operator/payouts?limit=50&cursor=${payoutNextCursor.value}`)
    payouts.value.push(...page.items)
    payoutNextCursor.value = page.next_cursor
    payoutHasMore.value = page.has_more ?? false
  } catch (cause: any) {
    error.value = cause.message ?? 'Unable to load more payouts'
  } finally {
    loadingMore.value = false
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

function isPending(status: string): boolean {
  return status === 'awaiting_confirmation'
}

function elapsed(ts?: string): string {
  if (!ts) return ''
  const ms = Date.now() - new Date(ts).getTime()
  if (Number.isNaN(ms) || ms < 0) return ''
  const h = Math.floor(ms / 3_600_000)
  const m = Math.floor((ms % 3_600_000) / 60_000)
  const s = Math.floor((ms % 60_000) / 1000)
  if (h > 0) return `waiting ${h}h ${m}m`
  if (m > 0) return `waiting ${m}m ${s}s`
  return `waiting ${s}s`
}

function waitingLabel(item: OperatorPayout): string {
  if (!isPending(item.status)) return ''
  return elapsed(item.submitted_at) || 'waiting…'
}

function epochLabel(item: OperatorPayout): string {
  if (item.epoch_from == null) return ''
  if (item.epoch_to == null || item.epoch_to === item.epoch_from) return String(item.epoch_from)
  return `${item.epoch_from}–${item.epoch_to}`
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
          <option value="">All states</option><option>failed</option><option>awaiting_confirmation</option><option>completed</option>
        </select>
        <input v-model="addressFilter" class="input" placeholder="Filter address" aria-label="Filter payout address" />
        <input v-model.number="epochFilter" type="number" min="0" class="input" placeholder="Filter epoch" aria-label="Filter payout epoch" />
      </div>
      <div v-if="filteredPayouts.length" class="table-wrap">
        <table><thead><tr><th>Status</th><th>Epoch</th><th>Recipient</th><th>Amount</th><th>Transaction</th><th>Waiting</th><th></th></tr></thead>
          <tbody><tr v-for="item in filteredPayouts" :key="item.hash">
            <td><StatusBadge :status="item.status || 'unknown'" /><span v-if="item.stuck" class="stuck-badge">stuck</span></td>
            <td class="epoch"><span v-if="epochLabel(item)">{{ epochLabel(item) }}</span><span v-else class="muted">—</span></td>
            <td class="address"><ExplorerLink kind="account" :value="item.address" /></td>
            <td><NimAmount :luna="item.amount ?? 0" /></td>
            <td class="hash"><ExplorerLink kind="transaction" :value="item.hash" /><span v-if="item.submitted_height" class="height">h{{ item.submitted_height }}</span></td>
            <td class="waiting"><span v-if="waitingLabel(item)">{{ waitingLabel(item) }}</span><span v-else class="muted">—</span></td>
            <td><button v-if="item.status === 'failed'" class="btn" @click="retryPayout(item.hash)">Retry</button></td>
          </tr></tbody></table>
      </div>
      <p v-else class="muted">No payout rows match these filters.</p>
      <button v-if="payoutHasMore" class="btn secondary" @click="loadMorePayouts" :disabled="loadingMore">Load more</button>
    </section>

    <section class="card">
      <h2>Validator action history</h2>
      <ul v-if="actions.length" class="action-list"><li v-for="item in actions" :key="item.id"><StatusBadge :status="item.state" /> {{ item.action }} <ExplorerLink v-if="item.tx_hash" kind="transaction" :value="item.tx_hash" label="tx" /><span v-if="item.error_summary" class="muted">{{ item.error_summary }}</span></li></ul>
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
.stuck-badge{margin-left:8px;padding:2px 8px;border-radius:999px;font-size:.7rem;font-weight:700;text-transform:uppercase;letter-spacing:.05em;background:var(--nimiq-red);color:#fff}
.height{margin-left:8px;font-size:.75rem;color:var(--nimiq-light-blue);white-space:nowrap}.waiting,.epoch{white-space:nowrap}
.destructive{background:var(--nimiq-red)}.btn.secondary{background:var(--surface-1);color:var(--app-text);border:1px solid var(--app-border);margin-top:16px}.btn.secondary:disabled{opacity:.55;cursor:not-allowed}.review-panel{margin-top:16px;padding:16px;background:var(--bg-muted);border-radius:10px}@media(max-width:640px){.filters,.action-buttons{align-items:stretch;flex-direction:column}.filters>*{max-width:none}}
</style>
