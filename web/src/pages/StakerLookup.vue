<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { apiGet } from '../api'
import StakerPosition from '../components/StakerPosition.vue'

const props = defineProps<{ address?: string }>()
const router = useRouter()
const input = ref(props.address ?? '')
const error = ref('')

interface StakerDetail {
  address: string
  stake_luna: number
  percentage: number
  payslips: { batch_number: number; amount_luna: number; status: string }[]
  transactions: { hash: string; amount_luna: number; status: string }[]
}

interface StakerHistory {
  address: string
  epochs: { epoch_number: number; stake: number; percentage: number }[]
  cumulative_reward_luna: number
}
const staker = ref<StakerDetail | null>(null)
const history = ref<StakerHistory | null>(null)

async function lookup(address: string) {
  error.value = ''
  staker.value = null
  history.value = null
  if (!address) return
  try {
    const [detail, hist] = await Promise.all([
      apiGet<StakerDetail>(`/api/stakers/${encodeURIComponent(address)}`),
      apiGet<StakerHistory>(`/api/stakers/${encodeURIComponent(address)}/history`)
    ])
    staker.value = detail
    history.value = hist
  } catch (e) {
    error.value = (e as Error).message
  }
}

function submit() {
  router.push(`/stakers/${encodeURIComponent(input.value)}`)
}

watch(() => props.address, (a) => { if (a) lookup(a) }, { immediate: true })
</script>

<template>
  <div class="card">
    <h1>Staker onboarding</h1>
    <p class="muted">Enter your Nimiq address to see your share and estimated rewards.</p>
    <form @submit.prevent="submit" class="form-row">
      <input v-model="input" placeholder="NQ.. .... .... .... .... .... .... .... ...." class="input address" />
      <button type="submit" class="btn">Look up</button>
    </form>
    <p v-if="error" class="error">{{ error }}</p>
    <div v-else-if="staker" class="results">
      <StakerPosition
        :position="{ address: staker.address, stake_luna: staker.stake_luna, percentage: staker.percentage }"
        :history="history"
        :export-url="`/api/stakers/${encodeURIComponent(staker.address)}/payslips.csv`"
      />
      <h2>Epoch history</h2>
      <table v-if="history?.epochs?.length">
        <thead><tr><th>Epoch</th><th>Stake</th><th>Share</th></tr></thead>
        <tbody>
          <tr v-for="e in history.epochs" :key="e.epoch_number">
            <td>{{ e.epoch_number }}</td>
            <td>{{ e.stake }}</td>
            <td>{{ e.percentage.toFixed(2) }}%</td>
          </tr>
        </tbody>
      </table>
      <h2>Payouts</h2>
      <ul>
        <li v-for="p in staker.payslips" :key="p.batch_number">
          batch {{ p.batch_number }}: {{ p.amount_luna }} luna — <span class="badge">{{ p.status }}</span>
        </li>
      </ul>
    </div>
  </div>
</template>

<style scoped>
.form-row {
  display: flex;
  gap: var(--space-12);
  margin-bottom: var(--space-24);
}
.form-row .input { flex: 1; }
.results h2 { margin-top: var(--space-24); }
ul { list-style: none; padding: 0; margin: 0; }
li { padding: var(--space-12) 0; border-bottom: 1px solid var(--border); }
li:last-child { border-bottom: none; }
</style>
