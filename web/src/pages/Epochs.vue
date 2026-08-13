<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { apiGet } from '../api'

interface Epoch {
  number: number
  num_stakers: number
  balance_luna: number
  status: string
}

const epochs = ref<Epoch[]>([])
const error = ref('')

onMounted(async () => {
  try {
    epochs.value = await apiGet<Epoch[]>('/api/epochs')
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <h1>Epochs</h1>
  <p v-if="error" class="error">{{ error }}</p>
  <table v-else>
    <thead><tr><th>Epoch</th><th>Status</th><th>Stakers</th></tr></thead>
    <tbody>
      <tr v-for="e in epochs" :key="e.number">
        <td><RouterLink :to="`/epochs/${e.number}`">{{ e.number }}</RouterLink></td>
        <td>{{ e.status }}</td>
        <td>{{ e.num_stakers }}</td>
      </tr>
    </tbody>
  </table>
</template>
