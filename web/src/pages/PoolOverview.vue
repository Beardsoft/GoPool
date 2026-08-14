<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiGet } from '../api'
import { useEvents } from '../composables/useEvents'
import NimAmount from '../components/ui/NimAmount.vue'
import type { PoolStatus } from '../types/api'

const pool = ref<PoolStatus | null>(null)
const error = ref('')
const lastEvent = ref<string>('')

onMounted(async () => {
  try {
    pool.value = await apiGet<PoolStatus>('/api/pool')
  } catch (e) {
    error.value = (e as Error).message
  }
})

const { events } = useEvents()
events.value.forEach(e => {
  if (e.type === 'epoch_started') {
    lastEvent.value = `Epoch ${e.data?.epoch} started`
    if (pool.value) {
      pool.value.current_epoch = e.data?.epoch
      pool.value.num_stakers = e.data?.numStakers ?? pool.value.num_stakers
    }
  }
  if (e.type === 'checkpoint_reward') {
    lastEvent.value = `Reward batch ${e.data?.batch}`
  }
})
</script>

<template>
  <section v-if="pool || error" class="card">
    <h1>Pool status</h1>
    <p v-if="lastEvent" class="muted">Live: {{ lastEvent }}</p>
    <div class="grid" v-if="pool">

      <div class="stat">
        <div class="label">Current epoch</div>
        <div class="value">{{ pool.current_epoch }} <span class="muted">({{ pool.epoch_status }})</span></div>
      </div>
      <div class="stat">
        <div class="label">Stakers</div>
        <div class="value">{{ pool.num_stakers }}</div>
      </div>
      <div class="stat">
        <div class="label">Total delegated stake</div>
        <div class="value"><NimAmount :luna="pool.total_stake_luna" /></div>
      </div>
      <div class="stat">
        <div class="label">Cumulative rewards paid</div>
        <div class="value"><NimAmount :luna="pool.total_rewards_luna" /></div>
      </div>
      <div class="stat">
        <div class="label">Pool fee</div>
        <div class="value">{{ (pool.pool_fee_percentage * 100).toFixed(2) }}%</div>
      </div>
    </div>
    <div class="grid skeleton" v-else>
      <div v-for="i in 5" :key="i" class="stat skeleton-item"></div>
    </div>
  </section>
  <p v-if="error" class="error">{{ error }}</p>
  <div v-if="!pool && !error" class="card">
    <div class="skeleton-title"></div>
    <div class="grid skeleton">
      <div v-for="i in 5" :key="i" class="stat skeleton-item"></div>
    </div>
  </div>
</template>

<style scoped>
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-16);
}
.stat {
  padding: var(--space-16);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--bg);
}
.label {
  font-size: 0.8rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--text-60);
  margin-bottom: var(--space-8);
}
.value {
  font-size: 1.25rem;
  font-weight: 700;
}
.skeleton-item { height: 80px; background: var(--bg-muted); border-radius: var(--radius-sm); animation: pulse 1.5s infinite }
.skeleton-title { height: 28px; width: 40%; background: var(--bg-muted); border-radius: var(--radius-sm); margin-bottom: var(--space-16); animation: pulse 1.5s infinite }
@keyframes pulse { 0% { opacity:.6 } 50% { opacity:.3 } 100% { opacity:.6 } }
</style>
