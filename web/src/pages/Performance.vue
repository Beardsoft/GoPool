<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { apiGet } from '../api'
import { RewardPoint } from '../types/api'
import { formatNim } from '../utils/format'
import RewardChart from '../components/RewardChart.vue'
import { RouterLink } from 'vue-router'

const rewards = ref<RewardPoint[]>([])
const range = ref<'20e' | '90d' | 'all'>('20e')
const error = ref('')

onMounted(async () => {
  try {
    const data = await apiGet<RewardPoint[]>('/api/pool/rewards')
    rewards.value = data
  } catch (e) {
    error.value = (e as Error).message
  }
})

const filtered = computed(() => {
  if (range.value === 'all') return rewards.value
  if (range.value === '20e') return rewards.value.slice(-20)
  return rewards.value.slice(-90)
})

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
        <thead><tr><th>Epoch</th><th>Total</th><th>Fee</th><th>Batches</th></tr></thead>
        <tbody>
          <tr v-for="p in filtered" :key="p.epoch_number">
            <td><RouterLink :to="`/epochs/${p.epoch_number}`">{{ p.epoch_number }}</RouterLink></td>
            <td>{{ formatNim(p.total_amount) }} NIM</td>
            <td>{{ formatNim(p.total_fee) }} NIM</td>
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
