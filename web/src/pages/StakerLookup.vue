<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, computed } from 'vue'
import { useRouter, RouterLink } from 'vue-router'
import { apiGet, apiPost } from '../api'
import { signStakingTransaction } from '../hub'
import Chart from 'chart.js/auto'
import NimAmount from '../components/ui/NimAmount.vue'
import ExplorerLink from '../components/ui/ExplorerLink.vue'
import StatusBadge from '../components/ui/StatusBadge.vue'
import EmptyState from '../components/ui/EmptyState.vue'
import SkeletonBlock from '../components/ui/SkeletonBlock.vue'
import { formatNim } from '../utils/format'
import { useSession } from '../composables/useSession'

const { signedIn, address: sessionAddress, login: sessionLogin } = useSession()
const props = defineProps<{ address?: string }>()
const router = useRouter()
const input = ref(props.address ?? '')
const error = ref('')
const loading = ref(false)

const lookedUp = ref('')
const noStake = ref(false)
const stakeAmount = ref('')
const staking = ref(false)
const stakeError = ref('')
const stakeTxHash = ref('')
const minStakeLuna = ref(0)
const balanceLuna = ref(0)

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

interface EpochRow {
  epoch_number: number
  stake_luna: number
  percentage: number
  reward_luna: number
}

interface StakerHistory {
  address: string
  epochs: EpochRow[]
  cumulative_reward_luna: number
}

const staker = ref<StakerDetail | null>(null)
const history = ref<StakerHistory | null>(null)
const chartEl = ref<HTMLCanvasElement | null>(null)
let chart: Chart | null = null

const epochPage = ref(0)
const epochPageSize = 20
const pagedEpochs = computed(() => {
  const rows = history.value?.epochs ?? []
  const start = epochPage.value * epochPageSize
  return rows.slice(start, start + epochPageSize)
})
const epochPages = computed(() => Math.max(1, Math.ceil((history.value?.epochs?.length ?? 0) / epochPageSize)))

const payoutPage = ref(0)
const payoutPageSize = 25
const pagedPayouts = computed(() => {
  const rows = staker.value?.payslips ?? []
  const start = payoutPage.value * payoutPageSize
  return rows.slice(start, start + payoutPageSize)
})
const payoutPages = computed(() => Math.max(1, Math.ceil((staker.value?.payslips?.length ?? 0) / payoutPageSize)))

const csvUrl = computed(() =>
  staker.value ? `/api/stakers/${encodeURIComponent(staker.value.address)}/payslips.csv` : ''
)

function renderChart() {
  if (!chartEl.value || !history.value?.epochs?.length) return
  if (chart) chart.destroy()
  const dark = document.documentElement.dataset.theme === 'dark'
  const palette = {
    text: dark ? 'rgba(255, 255, 255, 0.68)' : 'rgba(31, 35, 72, 0.68)',
    grid: dark ? 'rgba(255, 255, 255, 0.10)' : 'rgba(31, 35, 72, 0.10)',
    reward: dark ? '#0CA6FE' : '#0582CA',
  }
  const epochs = [...history.value.epochs].reverse()
  const labels = epochs.map((e) => `Epoch ${e.epoch_number}`)
  const rewards = epochs.map((e) => e.reward_luna / 100000)
  chart = new Chart(chartEl.value, {
    type: 'line',
    data: {
      labels,
      datasets: [{
        label: 'Reward (NIM)',
        data: rewards,
        tension: 0.3,
        borderWidth: 2,
        borderColor: palette.reward,
        backgroundColor: palette.reward,
        pointBackgroundColor: palette.reward,
      }]
    },
    options: {
      responsive: true,
      scales: {
        x: { ticks: { color: palette.text, maxTicksLimit: 8 }, grid: { color: palette.grid } },
        y: {
          beginAtZero: true,
          ticks: { color: palette.text, callback: (v) => formatNim(Number(v)) },
          grid: { color: palette.grid },
        },
      },
      plugins: {
        legend: { display: false },
        tooltip: {
          callbacks: {
            label: (ctx) => `${formatNim(Number(ctx.parsed.y))} NIM`,
          },
        },
      },
    },
  })
}

async function lookup(address: string) {
  error.value = ''
  staker.value = null
  history.value = null
  noStake.value = false
  lookedUp.value = address
  stakeTxHash.value = ''
  stakeError.value = ''
  epochPage.value = 0
  payoutPage.value = 0
  loading.value = true
  try {
    const [detail, hist] = await Promise.all([
      apiGet<StakerDetail>(`/api/stakers/${encodeURIComponent(address)}`),
      apiGet<StakerHistory>(`/api/stakers/${encodeURIComponent(address)}/history`)
    ])
    staker.value = detail
    history.value = hist
    setTimeout(renderChart, 0)
  } catch (e) {
    // A 404 means this address has no stake in the pool yet — not an error.
    if ((e as { status?: number }).status === 404) noStake.value = true
    else error.value = (e as { message?: string }).message ?? 'lookup failed'
  } finally {
    loading.value = false
  }
}

function submit() {
  const value = input.value.trim()
  if (value) router.push(`/stakers/${encodeURIComponent(value)}`)
}

const isOwn = computed(() => sessionAddress.value !== '' && staker.value?.address === sessionAddress.value)
const noStakeIsOwn = computed(() => sessionAddress.value !== '' && lookedUp.value === sessionAddress.value)

async function signIn() {
  error.value = ''
  try {
    await sessionLogin()
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function startStaking() {
  stakeError.value = ''
  stakeTxHash.value = ''
  const amountLuna = Math.round((Number(stakeAmount.value) || 0) * 100_000)
  if (amountLuna <= 0) {
    stakeError.value = 'Enter an amount greater than zero'
    return
  }
  staking.value = true
  try {
    const quote = await apiPost<{
      tx: string; amount_luna: number; fee_luna: number
      min_stake_luna: number; balance_luna: number
      sender: string; delegate: string; validity_start_height: number
    }>('/api/stake/quote', { amount_luna: amountLuna })
    minStakeLuna.value = quote.min_stake_luna
    balanceLuna.value = quote.balance_luna
    const signedTx = await signStakingTransaction(quote.sender, quote.tx)
    const res = await apiPost<{ tx_hash: string }>('/api/stake/submit', {
      signed_tx: signedTx,
      amount_luna: quote.amount_luna,
      fee_luna: quote.fee_luna,
      validity_start_height: quote.validity_start_height,
    })
    stakeTxHash.value = res.tx_hash
  } catch (e) {
    stakeError.value = (e as { message?: string }).message ?? 'stake failed'
  } finally {
    staking.value = false
  }
}

watch([() => props.address, () => sessionAddress.value], ([routeAddr, mine]) => {
  if (routeAddr) {
    lookup(routeAddr)
    return
  }
  if (mine) lookup(mine)
}, { immediate: true })
onMounted(() => {})
onUnmounted(() => { if (chart) chart.destroy() })
</script>

<template>
  <div class="staker-page">
    <section v-if="!props.address && !staker && !noStake" data-section="staker-lookup" class="card lookup-card">
      <p class="section-kicker">Find your stake</p>
      <h1>See exactly what your stake is doing.</h1>
      <p class="muted">Enter your Nimiq address to inspect delegated stake, pool share, rewards, and payout history.</p>
      <form class="lookup-form" @submit.prevent="submit">
        <label for="stake-address">Nimiq address</label>
        <div class="lookup-control">
          <input id="stake-address" v-model="input" aria-label="Nimiq address" autocomplete="off" placeholder="NQ00 …" />
          <button type="submit" class="btn">View position</button>
        </div>
        <small>No wallet connection required for public lookup.</small>
      </form>
    </section>

    <p v-else-if="error" class="error" role="alert">{{ error }}</p>

    <section v-else-if="noStake" class="card lookup-card" data-section="no-stake">
      <template v-if="stakeTxHash">
        <p class="section-kicker">Stake submitted</p>
        <h1>Your stake is on its way.</h1>
        <p class="muted">Your delegation was signed in your wallet and broadcast. It appears in your position once the pool processes the next epoch.</p>
        <ExplorerLink kind="transaction" :value="stakeTxHash" label="View transaction on explorer" />
      </template>
      <form v-else-if="noStakeIsOwn" class="stake-form" @submit.prevent="startStaking">
        <p class="section-kicker">Start staking</p>
        <h1>Delegate NIM to the pool.</h1>
        <p class="muted">You're not staking yet. Delegate NIM to start earning a share of pool rewards.</p>
        <label for="stake-amount">Amount (NIM)</label>
        <div class="lookup-control">
          <input id="stake-amount" v-model="stakeAmount" type="number" min="0" step="any" inputmode="decimal" aria-label="Amount in NIM" placeholder="e.g. 100" />
          <button type="submit" class="btn" :disabled="staking">{{ staking ? 'Waiting for wallet…' : 'Delegate to pool' }}</button>
        </div>
        <p v-if="minStakeLuna" class="muted">Minimum stake {{ formatNim(minStakeLuna) }} NIM · Available {{ formatNim(balanceLuna) }} NIM</p>
        <p v-if="stakeError" class="error" role="alert">{{ stakeError }}</p>
      </form>
      <template v-else>
        <p class="section-kicker">No stake found</p>
        <h1>This address isn't staking with us.</h1>
        <p class="muted">No delegated stake was found for this address in this pool.</p>
        <button v-if="!signedIn" class="btn" @click="signIn">Log in to stake</button>
      </template>
    </section>

    <template v-else-if="staker">
      <header class="staker-header">
        <div>
          <p class="section-kicker">Your position</p>
          <h1>Staker position</h1>
          <p class="address-line">
            <ExplorerLink kind="account" :value="staker.address" />
            <span v-if="isOwn" class="own-badge">Your stake</span>
          </p>
          <RouterLink to="/me" class="btn cta-manage">{{ signedIn ? 'Manage your stake' : 'Log in to manage your stake' }}</RouterLink>
        </div>
        <a :href="csvUrl" download class="btn">Download payslips CSV</a>
      </header>

      <section class="stat-grid" aria-label="Position summary">
        <article>
          <p>Delegated stake</p>
          <strong><NimAmount :luna="staker.stake_luna" /></strong>
          <small>Currently secured by this address</small>
        </article>
        <article>
          <p>Pool share</p>
          <strong>{{ staker.percentage.toFixed(2) }}%</strong>
          <small>Of total delegated stake</small>
        </article>
        <article>
          <p>Cumulative rewards</p>
          <strong><NimAmount :luna="history?.cumulative_reward_luna ?? 0" /></strong>
          <small>Earned across all epochs</small>
        </article>
        <article>
          <p>Payouts</p>
          <strong>{{ staker.payslips.length }}</strong>
          <small>On-chain payout transactions</small>
        </article>
      </section>

      <section v-if="history?.epochs?.length" class="card" data-section="reward-chart">
        <h2>Rewards per epoch</h2>
        <div class="chart-wrap">
          <canvas ref="chartEl" role="img" aria-label="Line chart of rewards per epoch in NIM"></canvas>
        </div>
      </section>

      <section class="card" data-section="epoch-history">
        <h2>Epoch history</h2>
        <template v-if="history?.epochs?.length">
          <table>
            <thead><tr><th>Epoch</th><th>Stake</th><th>Share</th><th>Reward</th></tr></thead>
            <tbody>
              <tr v-for="e in pagedEpochs" :key="e.epoch_number">
                <td>{{ e.epoch_number }}</td>
                <td><NimAmount :luna="e.stake_luna" /></td>
                <td>{{ e.percentage.toFixed(2) }}%</td>
                <td><NimAmount :luna="e.reward_luna" /></td>
              </tr>
            </tbody>
          </table>
          <div class="pagination-controls">
            <button class="btn btn-ghost" :disabled="epochPage === 0" @click="epochPage--">Previous</button>
            <span>Page {{ epochPage + 1 }} of {{ epochPages }}</span>
            <button class="btn btn-ghost" :disabled="epochPage + 1 >= epochPages" @click="epochPage++">Next</button>
          </div>
        </template>
        <EmptyState v-else title="No epoch history yet" description="Epoch records appear here once the pool has processed rewards for this address." />
      </section>

      <section class="card" data-section="payouts">
        <h2>Payouts</h2>
        <table v-if="staker.payslips.length">
          <thead><tr><th>Batch</th><th>Epoch</th><th>Amount</th><th>Status</th><th>Transaction</th></tr></thead>
          <tbody>
            <tr v-for="p in pagedPayouts" :key="p.batch_number">
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
        <div v-if="staker.payslips.length > payoutPageSize" class="pagination-controls">
          <button class="btn btn-ghost" :disabled="payoutPage === 0" @click="payoutPage--">Previous</button>
          <span>Page {{ payoutPage + 1 }} of {{ payoutPages }}</span>
          <button class="btn btn-ghost" :disabled="payoutPage + 1 >= payoutPages" @click="payoutPage++">Next</button>
        </div>
        <EmptyState v-if="!staker.payslips.length" title="No payouts yet" description="Payouts appear here once your accumulated rewards cross the pool's minimum payout threshold." />
      </section>
    </template>

    <section v-else-if="loading" class="card" aria-busy="true" aria-label="Loading staker position">
      <SkeletonBlock width="220px" height="30px" />
      <div class="stat-grid">
        <SkeletonBlock v-for="i in 4" :key="i" height="92px" />
      </div>
    </section>
  </div>
</template>

<style scoped>
.staker-page { display: grid; gap: var(--space-24); }
.section-kicker { margin-bottom: 12px; color: var(--nimiq-light-blue); font-weight: 700; }
.lookup-card { max-width: 720px; }
.lookup-form { margin-top: var(--space-24); }
.lookup-form label { display: block; margin-bottom: 9px; font-size: .82rem; font-weight: 700; }
.lookup-control { display: flex; gap: 10px; }
.lookup-control input { flex: 1; min-width: 0; font-family: var(--font-mono); }
.lookup-control .btn { flex: 0 0 auto; }
.lookup-form small { display: block; margin-top: 10px; color: var(--app-faint); }
.stake-form { margin-top: var(--space-24); }
.stake-form label { display: block; margin-bottom: 9px; font-size: .82rem; font-weight: 700; }
.stake-form .muted { margin-top: 12px; }

.staker-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--space-24);
  flex-wrap: wrap;
}
.address-line { margin-bottom: 0; }
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
.cta-manage {
  display: inline-block;
  margin-top: var(--space-12);
  color: var(--nimiq-light-blue);
  border: 1px solid color-mix(in srgb, var(--nimiq-light-blue) 40%, transparent);
  box-shadow: none;
}
.cta-manage:hover { box-shadow: none; }

.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--space-16);
}
.stat-grid article {
  padding: 20px;
  border-radius: 14px;
  background: var(--surface-1);
  box-shadow: var(--shadow-subtle);
}
.stat-grid p { margin: 0 0 8px; color: var(--app-faint); font-size: .78rem; font-weight: 700; }
.stat-grid strong { display: block; font-size: 1.4rem; letter-spacing: -.02em; }
.stat-grid small { display: block; margin-top: 6px; color: var(--app-faint); font-size: .78rem; }

.chart-wrap { height: 300px; }

.pagination-controls {
  display: flex;
  align-items: center;
  gap: var(--space-12);
  margin-top: var(--space-16);
}
.pagination-controls span { color: var(--app-muted); font-size: .85rem; font-weight: 600; }
.btn-ghost {
  min-height: 36px;
  padding: 0 14px;
  color: var(--app-text);
  background: var(--surface-2);
  box-shadow: none;
}
.btn-ghost:hover { transform: none; box-shadow: none; }

@media (max-width: 900px) {
  .stat-grid { grid-template-columns: repeat(2, 1fr); }
}
@media (max-width: 620px) {
  .stat-grid { grid-template-columns: 1fr; }
  .lookup-control { flex-direction: column; }
  .staker-header { align-items: flex-start; }
}
</style>
