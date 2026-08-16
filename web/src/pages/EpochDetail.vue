<script setup lang="ts">
import { onMounted, ref, watch, computed } from 'vue'
import { apiGet } from '../api'
import ExplorerLink from '../components/ui/ExplorerLink.vue'
import NimAmount from '../components/ui/NimAmount.vue'
import type { EpochDetail, RewardBatch } from '../types/api'

const props = defineProps<{ number: string }>()

const epoch = ref<EpochDetail | null>(null)
const rewards = ref<RewardBatch[]>([])
const error = ref('')
const stakerPage = ref(0)
const stakerPageSize = 50
const pagedStakers = computed(() => {
  if (!epoch.value?.stakers) return []
  const start = stakerPage.value * stakerPageSize
  return epoch.value.stakers.slice(start, start + stakerPageSize)
})

async function load() {
  error.value = ''
  epoch.value = null
  rewards.value = []
  try {
    epoch.value = await apiGet<EpochDetail>(`/api/epochs/${props.number}`)
    rewards.value = await apiGet<RewardBatch[]>(`/api/epochs/${props.number}/rewards`)
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(load)
watch(() => props.number, load)
</script>

<template>
  <div class="card">
    <p v-if="error" class="error">{{ error }}</p>
    <div v-else-if="epoch">
      <h1>Epoch {{ epoch.number }}</h1>
      <p class="muted">{{ epoch.status }} — {{ epoch.num_stakers }} stakers</p>

      <h2 style="margin-top:24px">Rewards</h2>
      <table v-if="rewards.length">
        <thead><tr><th>Batch</th><th>Amount</th><th>Pool fee</th><th>Stakers</th></tr></thead>
        <tbody>
          <tr v-for="r in rewards" :key="r.batch_number">
            <td>{{ r.batch_number }}</td>
            <td><NimAmount :luna="r.amount_luna" /></td>
            <td><NimAmount :luna="r.pool_fee_luna" /></td>
            <td>{{ r.num_stakers }}</td>
          </tr>
        </tbody>
      </table>
      <p v-else class="muted">No rewards recorded yet.</p>

      <h2 style="margin-top:24px">Stakers</h2>
      <div v-if="epoch.stakers.length" style="display:flex;gap:12px;align-items:center;margin-bottom:8px">
        <button class="btn" :disabled="stakerPage===0" @click="stakerPage--">Prev</button>
        <span>Page {{ stakerPage+1 }} / {{ Math.max(1, Math.ceil(epoch.stakers.length / stakerPageSize)) }}</span>
        <button class="btn" :disabled="(stakerPage+1)*stakerPageSize >= epoch.stakers.length" @click="stakerPage++">Next</button>
      </div>
      <table v-if="epoch.stakers.length">
        <thead><tr><th>Address</th><th>Stake</th><th>%</th></tr></thead>
        <tbody>
          <tr v-for="s in pagedStakers" :key="s.address">
            <td class="address"><ExplorerLink kind="account" :value="s.address" /></td>
            <td><NimAmount :luna="s.stake_luna" /></td>
            <td>{{ s.percentage.toFixed(2) }}%</td>
          </tr>
        </tbody>
      </table>
      <p v-else class="muted">No stakers recorded for this epoch.</p>
    </div>
  </div>
</template>
