<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiGet } from '../api'

interface PoolStatus {
  current_epoch: number
  epoch_status: string
  num_stakers: number
  total_stake_luna: number
  total_rewards_luna: number
  pool_fee_percentage: number
}

const pool = ref<PoolStatus | null>(null)
const error = ref('')

function nim(luna: number): string {
  return (luna / 100000).toLocaleString(undefined, { maximumFractionDigits: 5 })
}

onMounted(async () => {
  try {
    pool.value = await apiGet<PoolStatus>('/api/pool')
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <section v-if="pool">
    <h1>Pool status</h1>
    <dl>
      <dt>Epoch</dt><dd>{{ pool.current_epoch }} ({{ pool.epoch_status }})</dd>
      <dt>Stakers</dt><dd>{{ pool.num_stakers }}</dd>
      <dt>Total delegated stake</dt><dd>{{ nim(pool.total_stake_luna) }} NIM</dd>
      <dt>Cumulative rewards paid</dt><dd>{{ nim(pool.total_rewards_luna) }} NIM</dd>
      <dt>Pool fee</dt><dd>{{ (pool.pool_fee_percentage * 100).toFixed(2) }}%</dd>
    </dl>
  </section>
  <p v-else-if="error" class="error">{{ error }}</p>
  <p v-else>Loading…</p>
</template>
