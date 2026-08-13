<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { apiGet } from '../api'

const props = defineProps<{ number: string }>()

interface Staker {
  address: string
  stake_luna: number
  percentage: number
}
interface EpochDetail {
  number: number
  status: string
  num_stakers: number
  balance_luna: number
  stakers: Staker[]
}

const epoch = ref<EpochDetail | null>(null)
const error = ref('')

async function load() {
  error.value = ''
  epoch.value = null
  try {
    epoch.value = await apiGet<EpochDetail>(`/api/epochs/${props.number}`)
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(load)
watch(() => props.number, load)
</script>

<template>
  <p v-if="error" class="error">{{ error }}</p>
  <section v-else-if="epoch">
    <h1>Epoch {{ epoch.number }}</h1>
    <p>{{ epoch.status }} — {{ epoch.num_stakers }} stakers</p>
    <table>
      <thead><tr><th>Address</th><th>Stake (luna)</th><th>%</th></tr></thead>
      <tbody>
        <tr v-for="s in epoch.stakers" :key="s.address">
          <td class="address">{{ s.address }}</td>
          <td>{{ s.stake_luna }}</td>
          <td>{{ s.percentage.toFixed(2) }}%</td>
        </tr>
      </tbody>
    </table>
  </section>
</template>
