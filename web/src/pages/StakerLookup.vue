<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { apiGet } from '../api'

const props = defineProps<{ address?: string }>()
const router = useRouter()
const input = ref(props.address ?? '')
const error = ref('')

interface StakerDetail {
  address: string
  stake_luna: number
  percentage: number
  payslips: { batch_number: number; amount_luna: number; status: string }[]
  transactions: { hash: string; amount_luna: number; status: string }[]
}
const staker = ref<StakerDetail | null>(null)

async function lookup(address: string) {
  error.value = ''
  staker.value = null
  if (!address) return
  try {
    staker.value = await apiGet<StakerDetail>(`/api/stakers/${encodeURIComponent(address)}`)
  } catch (e) {
    error.value = (e as Error).message
  }
}

function submit() {
  router.push(`/stakers/${encodeURIComponent(input.value)}`)
}

watch(() => props.address, (a) => { if (a) lookup(a) }, { immediate: true })
</script>

<template>
  <h1>Look up a staker</h1>
  <form @submit.prevent="submit">
    <input v-model="input" placeholder="NQ.. .... .... .... .... .... .... .... ...." class="address" />
    <button type="submit">Look up</button>
  </form>
  <p v-if="error" class="error">{{ error }}</p>
  <section v-else-if="staker">
    <p>Stake: {{ staker.stake_luna }} luna ({{ staker.percentage.toFixed(2) }}%)</p>
    <h2>Payouts</h2>
    <ul>
      <li v-for="p in staker.payslips" :key="p.batch_number">
        batch {{ p.batch_number }}: {{ p.amount_luna }} luna — {{ p.status }}
      </li>
    </ul>
  </section>
</template>
