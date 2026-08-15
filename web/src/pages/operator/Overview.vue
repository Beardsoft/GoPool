<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiGet } from '../../api'
import { useLiveStatus } from '../../composables/useLiveStatus'
import type { OperatorOverview } from '../../types/api'
import NimAmount from '../../components/ui/NimAmount.vue'
import TelemetryChart from '../../components/TelemetryChart.vue'
import { formatNim } from '../../utils/format'

const overview = ref<OperatorOverview | null>(null)
const loading = ref(true)
const error = ref('')

const { state, lastEventAt, reconnect } = useLiveStatus()

async function load() {
  try {
    overview.value = await apiGet<OperatorOverview>('/api/operator/overview')
    error.value = ''
  } catch (e: any) {
    error.value = e.message ?? 'Failed to load'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="operator-overview">
    <div class="global-status">
      <span>State: {{ state }}</span>
      <span v-if="state === 'paused'">Stale data</span>
      <button @click="reconnect">Reconnect</button>
    </div>

    <div v-if="overview" class="metrics">
      <div class="metric">Stake: <NimAmount :luna="overview.metrics.total_stake_luna" /></div>
      <div class="metric">Rewards: <NimAmount :luna="overview.metrics.total_rewards_luna" /></div>
      <div class="metric">Stakers: {{ overview.metrics.num_stakers }}</div>
      <div class="metric">Runway: {{ overview.metrics.wallet_runway_days }} days</div>
    </div>

    <section data-section="attention" class="section">
      <h2>Attention</h2>
      <ul v-if="overview?.attention?.length">
        <li v-for="e in overview.attention" :key="e.id">{{ e.summary }}</li>
      </ul>
      <p v-else>No attention required</p>
    </section>

    <section data-section="validator" class="section">
      <h2>Validator</h2>
      <p>{{ overview?.validator?.address }}</p>
      <p>State: {{ overview?.validator?.state }}</p>
    </section>

    <section data-section="telemetry" class="section">
      <h2>Telemetry</h2>
      <TelemetryChart v-if="overview?.telemetry_points?.length" :points="overview.telemetry_points" />
      <p v-else>No data</p>
    </section>

    <section data-section="activity" class="section">
      <h2>Activity</h2>
      <ul>
        <li v-for="e in overview?.recent_activity" :key="e.id">{{ e.summary }}</li>
      </ul>
    </section>

    <p v-if="error">{{ error }}</p>
    <p v-if="loading">Loading...</p>
  </div>
</template>

<style scoped>
.operator-overview { display: grid; gap: 16px; }
.metrics { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; }
.section { border: 1px solid var(--border); padding: 12px; border-radius: 8px; }
.global-status { display: flex; gap: 12px; align-items: center; }
</style>
