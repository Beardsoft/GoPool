<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { apiGet } from '../api'
import { PoolStatus, RewardPoint } from '../types/api'
import { formatNim } from '../utils/format'
import RewardChart from '../components/RewardChart.vue'
import { RouterLink } from 'vue-router'

const rewards = ref<RewardPoint[]>([])
const totalStakeLuna = ref(0)
const numStakers = ref(0)
const range = ref<'20e' | '90d' | 'all'>('20e')
const error = ref('')

onMounted(async () => {
  try {
    const [data, pool] = await Promise.all([
      apiGet<RewardPoint[]>('/api/pool/rewards'),
      apiGet<PoolStatus>('/api/pool'),
    ])
    rewards.value = data
    totalStakeLuna.value = pool.total_stake_luna
    numStakers.value = pool.num_stakers
  } catch (e) {
    error.value = (e as Error).message
  }
})

// Chronological (oldest → newest) for the chart and range windowing.
const filtered = computed(() => {
  const sorted = [...rewards.value].sort((a, b) => a.epoch_number - b.epoch_number)
  if (range.value === 'all') return sorted
  if (range.value === '20e') return sorted.slice(-20)
  return sorted.slice(-90)
})

// Newest first in the table so recent epochs aren't buried under 0…N.
const epochList = computed(() => [...filtered.value].reverse())

const totalRewardsLuna = computed(() => filtered.value.reduce((a, r) => a + r.total_amount, 0))
const totalFeesLuna = computed(() => filtered.value.reduce((a, r) => a + r.total_fee, 0))
</script>

<template>
  <section class="card">
    <h1>Performance</h1>
    <p v-if="error" class="error">{{ error }}</p>
    <div v-else>
      <div class="stats">
        <div class="stat">
          <div class="label">Epochs shown</div>
          <div class="value">{{ filtered.length }}</div>
        </div>
        <div class="stat">
          <div class="label">Total stake</div>
          <div class="value">{{ formatNim(totalStakeLuna) }} NIM</div>
        </div>
        <div class="stat">
          <div class="label">Stakers</div>
          <div class="value">{{ numStakers }}</div>
        </div>
        <div class="stat">
          <div class="label">Total rewards</div>
          <div class="value">{{ formatNim(totalRewardsLuna) }} NIM</div>
        </div>
        <div class="stat">
          <div class="label">Total fees</div>
          <div class="value">{{ formatNim(totalFeesLuna) }} NIM</div>
        </div>
      </div>
      <div class="range-controls">
        <button :class="{ active: range === '20e' }" @click="range='20e'">20 epochs</button>
        <button :class="{ active: range === '90d' }" @click="range='90d'">90 days</button>
        <button :class="{ active: range === 'all' }" @click="range='all'">All</button>
      </div>
      <RewardChart :points="filtered" :range="range" />
      <h2 class="section-title">Epoch list</h2>
      <table>
        <thead><tr><th>Epoch</th><th>Total</th><th>Fee</th><th>Stakers</th><th>Batches</th></tr></thead>
        <tbody>
          <tr v-for="p in epochList" :key="p.epoch_number">
            <td><RouterLink :to="`/epochs/${p.epoch_number}`">{{ p.epoch_number }}</RouterLink></td>
            <td>{{ formatNim(p.total_amount) }} NIM</td>
            <td>{{ formatNim(p.total_fee) }} NIM</td>
            <td>{{ p.num_stakers ?? 0 }}</td>
            <td>{{ p.batches }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: var(--space-16); margin-bottom: var(--space-16); }
.stat { padding: var(--space-12); border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--bg); }
.label { font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-60); margin-bottom: var(--space-4); }
.value { font-weight: 700; }
.section-title { margin: var(--space-24) 0 var(--space-12); font-size: 1.05rem; }
.range-controls { margin: var(--space-12) 0; display: flex; gap: var(--space-8); }
.range-controls button { padding: var(--space-8) var(--space-12); border: 1px solid var(--border); background: var(--bg); cursor: pointer; }
.range-controls button.active { background: var(--primary); color: white; }
.error { color: red; }
</style>
