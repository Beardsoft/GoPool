<script setup lang="ts">
import { formatNim } from '../utils/format'

interface StakerPositionData {
  address: string
  stake_luna: number
  percentage: number
}

interface StakerHistoryData {
  cumulative_reward_luna?: number
  epochs?: { epoch_number: number; stake: number; percentage: number }[]
}

defineProps<{
  position: StakerPositionData
  history?: StakerHistoryData | null
  exportUrl: string
}>()
</script>

<template>
  <div class="position">
    <div class="stat">
      <div class="label">Stake</div>
      <div class="value">{{ formatNim(position.stake_luna) }} NIM</div>
    </div>
    <div class="stat">
      <div class="label">Share</div>
      <div class="value">{{ position.percentage.toFixed(2) }}%</div>
    </div>
    <div v-if="history?.cumulative_reward_luna" class="stat">
      <div class="label">Cumulative rewards</div>
      <div class="value">{{ formatNim(history.cumulative_reward_luna) }} NIM</div>
    </div>
    <a :href="exportUrl" download class="btn">Download CSV</a>
  </div>
</template>

<style scoped>
.position { display: flex; flex-direction: column; gap: var(--space-16); }
.stat { padding: var(--space-16); border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--bg); }
.label { font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-60); margin-bottom: var(--space-8); }
.value { font-size: 1.25rem; font-weight: 700; }
</style>
