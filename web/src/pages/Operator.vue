<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiGet, apiPost } from '../api'
import { loginWithHub } from '../hub'
import ExplorerLink from '../components/ui/ExplorerLink.vue'

interface Health {
  last_processed_height: number
  chain_head: number
  stuck_payslips: { batch_number: number; address: string; amount_luna: number; status: string }[]
}

interface AuditLog {
  id: number
  action_type: string
  address: string
  amount: number
  fee: number
  kind: string
  status: string
  created_at: string
}

const health = ref<Health | null>(null)
const auditLogs = ref<AuditLog[]>([])
const authorized = ref(false)
const error = ref('')
const actionStatus = ref('')

async function loadHealth() {
  try {
    health.value = await apiGet<Health>('/api/operator/health')
    await loadAudit()
    authorized.value = true
    error.value = ''
  } catch (e) {
    authorized.value = false
    health.value = null
    error.value = (e as Error).message
  }
}

async function loadAudit() {
  try {
    auditLogs.value = await apiGet<AuditLog[]>('/api/operator/audit')
  } catch {}
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

async function approve(id: number) {
  error.value = ''
  try {
    await apiPost('/api/operator/audit/approve', { id })
    await loadAudit()
    actionStatus.value = 'Approved'
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function skip(id: number) {
  error.value = ''
  try {
    await apiPost('/api/operator/audit/skip', { id })
    await loadAudit()
    actionStatus.value = 'Skipped'
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(loadHealth)
</script>

<template>
  <div class="card">
    <h1>Operator panel</h1>
    <button v-if="!authorized" @click="login" class="btn">Log in with Nimiq Hub</button>
    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="actionStatus" class="muted">{{ actionStatus }}</p>
    <div v-if="health">
      <div class="grid" style="margin-top: var(--space-16);">
        <div class="stat">
          <div class="label">Daemon height</div>
          <div class="value">{{ health.last_processed_height }}</div>
        </div>
        <div class="stat">
          <div class="label">Chain head</div>
          <div class="value">{{ health.chain_head }}</div>
        </div>
        <div class="stat">
          <div class="label">Lag</div>
          <div class="value">{{ health.chain_head - health.last_processed_height }}</div>
        </div>
      </div>
      <h2>Stuck payslips</h2>
      <ul v-if="health.stuck_payslips.length">
        <li v-for="p in health.stuck_payslips" :key="p.address + p.batch_number">
          <span class="address"><ExplorerLink kind="account" :value="p.address" /></span> — batch {{ p.batch_number }}, {{ p.amount_luna }} luna <span class="badge">{{ p.status }}</span>
        </li>
      </ul>
      <p v-else class="muted">None.</p>
      <h2>Pending audit actions</h2>
      <table v-if="auditLogs.length" class="audit-table">
        <thead>
          <tr><th>ID</th><th>Address</th><th>Amount</th><th>Fee</th><th>Kind</th><th>Status</th><th>Created</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="log in auditLogs" :key="log.id">
            <td>{{ log.id }}</td>
            <td class="address"><ExplorerLink kind="account" :value="log.address" /></td>
            <td>{{ log.amount }}</td>
            <td>{{ log.fee }}</td>
            <td>{{ log.kind }}</td>
            <td>{{ log.status }}</td>
            <td>{{ log.created_at }}</td>
            <td>
              <button class="btn" @click="approve(log.id)">Approve</button>
              <button class="btn" @click="skip(log.id)" style="background: var(--nimiq-red);">Skip</button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="muted">No pending actions.</p>
      <h2>Validator actions</h2>
      <div class="actions">
        <button @click="trigger('deactivate')" class="btn" style="background: var(--nimiq-red);">Deactivate</button>
        <button @click="trigger('retire')" class="btn" style="background: var(--nimiq-gold); color:#000;">Retire</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px,1fr)); gap: var(--space-16); margin-bottom: var(--space-24); }
.stat { padding: var(--space-16); border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--bg); }
.label { font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-60); margin-bottom: var(--space-8); }
.value { font-size: 1.25rem; font-weight: 700; }
.actions { display: flex; gap: var(--space-12); margin-top: var(--space-12); }
ul { list-style: none; padding: 0; margin: 0; }
li { padding: var(--space-12) 0; border-bottom: 1px solid var(--border); }
li:last-child { border-bottom: none; }
.audit-table { width: 100%; border-collapse: collapse; margin-top: var(--space-12); }
.audit-table th, .audit-table td { padding: var(--space-8); border-bottom: 1px solid var(--border); text-align: left; font-size: 0.9rem; }
.audit-table th { color: var(--text-60); text-transform: uppercase; font-size: 0.75rem; }
.audit-table .btn { margin-right: var(--space-8); padding: 4px 8px; font-size: 0.8rem; }
</style>
