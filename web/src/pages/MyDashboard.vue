<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiGet } from '../api'
import { loginWithHub } from '../hub'

interface StakerDetail {
  address: string
  stake_luna: number
  percentage: number
  payslips: { batch_number: number; amount_luna: number; status: string }[]
}

const me = ref<StakerDetail | null>(null)
const loggedIn = ref(false)
const error = ref('')

async function load() {
  try {
    me.value = await apiGet<StakerDetail>('/api/me')
    loggedIn.value = true
  } catch {
    loggedIn.value = false
  }
}

async function login() {
  error.value = ''
  try {
    await loginWithHub()
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(load)
</script>

<template>
  <h1>My dashboard</h1>
  <button v-if="!loggedIn" @click="login">Log in with Nimiq Hub</button>
  <p v-if="error" class="error">{{ error }}</p>
  <section v-if="me">
    <p>Address: <span class="address">{{ me.address }}</span></p>
    <p>Stake: {{ me.stake_luna }} luna ({{ me.percentage.toFixed(2) }}%)</p>
    <ul>
      <li v-for="p in me.payslips" :key="p.batch_number">
        batch {{ p.batch_number }}: {{ p.amount_luna }} luna — {{ p.status }}
      </li>
    </ul>
  </section>
</template>
