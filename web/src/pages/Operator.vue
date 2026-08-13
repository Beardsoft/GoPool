<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiGet, apiPost } from '../api'
import { loginWithHub } from '../hub'

interface Health {
  last_processed_height: number
  chain_head: number
  stuck_payslips: { batch_number: number; address: string; amount_luna: number; status: string }[]
}

const health = ref<Health | null>(null)
const authorized = ref(false)
const error = ref('')
const actionStatus = ref('')

async function loadHealth() {
  try {
    health.value = await apiGet<Health>('/api/operator/health')
    authorized.value = true
  } catch {
    authorized.value = false
  }
}

async function login() {
  error.value = ''
  try {
    await loginWithHub()
    await loadHealth()
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function trigger(action: 'deactivate' | 'retire') {
  actionStatus.value = ''
  error.value = ''
  try {
    await apiPost(`/api/operator/validator/${action}`)
    actionStatus.value = `${action} requested — the daemon will submit it on its next tick.`
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(loadHealth)
</script>

<template>
  <h1>Operator panel</h1>
  <button v-if="!authorized" @click="login">Log in with Nimiq Hub</button>
  <p v-if="error" class="error">{{ error }}</p>
  <p v-if="actionStatus">{{ actionStatus }}</p>
  <section v-if="health">
    <p>Daemon height: {{ health.last_processed_height }} / chain head: {{ health.chain_head }}
      ({{ health.chain_head - health.last_processed_height }} behind)</p>
    <h2>Stuck payslips</h2>
    <ul v-if="health.stuck_payslips.length">
      <li v-for="p in health.stuck_payslips" :key="p.address + p.batch_number">
        {{ p.address }} — batch {{ p.batch_number }}, {{ p.amount_luna }} luna, {{ p.status }}
      </li>
    </ul>
    <p v-else>None.</p>
    <h2>Validator actions</h2>
    <button @click="trigger('deactivate')">Deactivate</button>
    <button @click="trigger('retire')">Retire</button>
  </section>
</template>
