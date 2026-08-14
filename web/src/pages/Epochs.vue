<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { apiGet } from '../api'
import NimAmount from '../components/ui/NimAmount.vue'
import type { EpochSummary } from '../types/api'

const epochs = ref<EpochSummary[]>([])
const error = ref('')

onMounted(async () => {
  try {
    epochs.value = await apiGet<EpochSummary[]>('/api/epochs')
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <div class="card">
    <h1>Epochs</h1>
    <p v-if="error" class="error">{{ error }}</p>
    <table v-else>
      <thead><tr><th>Epoch</th><th>Status</th><th>Stakers</th><th>Balance</th></tr></thead>
      <tbody>
        <tr v-for="e in epochs" :key="e.number">
          <td><RouterLink :to="`/epochs/${e.number}`">{{ e.number }}</RouterLink></td>
          <td><span class="badge">{{ e.status }}</span></td>
          <td>{{ e.num_stakers }}</td>
          <td><NimAmount :luna="e.balance_luna" /></td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
