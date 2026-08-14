<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiGet } from '../api'
import { loginWithHub } from '../hub'
import StakerPosition from '../components/StakerPosition.vue'

interface StakerDetail {
  address: string
  stake_luna: number
  percentage: number
  payslips: { batch_number: number; amount_luna: number; status: string }[]
}

interface StakerHistory {
  cumulative_reward_luna: number
}

const me = ref<StakerDetail | null>(null)
const history = ref<StakerHistory | null>(null)
const loggedIn = ref(false)
const error = ref('')

async function load() {
  try {
    const [detail, hist] = await Promise.all([
      apiGet<StakerDetail>('/api/me'),
      apiGet<StakerHistory>('/api/me/history')
    ])
    me.value = detail
    history.value = hist
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

function downloadPdf() {
  window.print()
}

onMounted(load)
</script>

<template>
  <div class="card">
    <h1>My dashboard</h1>
    <button v-if="!loggedIn" @click="login" class="btn">Log in with Nimiq Hub</button>
    <p v-if="error" class="error">{{ error }}</p>
    <div v-if="me">
      <p class="muted">Address: <span class="address">{{ me.address }}</span></p>
      <StakerPosition
        :position="{ address: me.address, stake_luna: me.stake_luna, percentage: me.percentage }"
        :history="history"
        export-url="/api/me/payslips.csv"
      />
      <h2 style="margin-top: var(--space-24);">Payslips</h2>
      <div style="display:flex; gap:8px; margin-bottom:12px">
        <button class="btn" @click="downloadPdf">Download PDF</button>
      </div>
      <ul>
        <li v-for="p in me.payslips" :key="p.batch_number">
          batch {{ p.batch_number }}: {{ p.amount_luna }} luna — <span class="badge">{{ p.status }}</span>
        </li>
      </ul>
    </div>
  </div>
</template>

<style scoped>
.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px,1fr)); gap: var(--space-16); }
.stat { padding: var(--space-16); border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--bg); }
.label { font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-60); margin-bottom: var(--space-8); }
.value { font-size: 1.25rem; font-weight: 700; }
ul { list-style: none; padding: 0; margin: 0; }
li { padding: var(--space-12) 0; border-bottom: 1px solid var(--border); }
li:last-child { border-bottom: none; }
</style>
