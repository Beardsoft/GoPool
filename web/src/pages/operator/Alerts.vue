<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiGet, apiPost } from '../../api'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import type { AlertChannelStatus, AlertDelivery, PageResponse } from '../../types/api'

const channels = ref<Record<string, AlertChannelStatus>>({})
const deliveries = ref<AlertDelivery[]>([])
const error = ref('')
const testing = ref('')

async function load() {
  try {
    const [settings, history] = await Promise.all([
      apiGet<{ channels: Record<string, AlertChannelStatus> }>('/api/operator/alerts'),
      apiGet<PageResponse<AlertDelivery>>('/api/operator/alerts/deliveries?limit=50'),
    ])
    channels.value = settings.channels
    deliveries.value = history.items
  } catch (cause: any) { error.value = cause.message ?? 'Unable to load alerts' }
}

async function testChannel(channel: string) {
  testing.value = channel
  try {
    const result = await apiPost<AlertDelivery>(`/api/operator/alerts/${channel}/test`, {})
    deliveries.value.unshift(result)
  } catch (cause: any) { error.value = cause.message ?? 'Delivery test failed' }
  finally { testing.value = '' }
}

onMounted(load)
</script>

<template>
  <main class="alerts-page">
    <header><p class="eyebrow">Delivery health</p><h1>Alerts</h1><p class="muted">Deployment-managed credentials are never shown here.</p></header>
    <p v-if="error" class="error">{{ error }}</p>
    <section class="channel-grid">
      <article v-for="(channel, name) in channels" :key="name" class="card">
        <div class="channel-heading"><h2>{{ name }}</h2><StatusBadge :status="channel.state" /></div>
        <p>{{ channel.destination_hint || 'No destination configured' }}</p>
        <button class="btn" :disabled="!channel.configured || testing === name" @click="testChannel(String(name))">Test delivery</button>
      </article>
    </section>
    <section class="card"><h2>Delivery history</h2>
      <div v-if="deliveries.length" class="table-wrap"><table><thead><tr><th>State</th><th>Channel</th><th>Type</th><th>Destination</th><th>Result</th></tr></thead>
        <tbody><tr v-for="item in deliveries" :key="item.id"><td><StatusBadge :status="item.state" /></td><td>{{ item.channel }}</td><td>{{ item.alert_type }}</td><td>{{ item.destination }}</td><td>{{ item.response_summary }}</td></tr></tbody></table></div>
      <p v-else class="muted">Delivery attempts will appear here.</p>
    </section>
  </main>
</template>

<style scoped>
.alerts-page{display:grid;gap:24px}.eyebrow{text-transform:uppercase;letter-spacing:.08em;font-size:.75rem;font-weight:700;color:var(--nimiq-light-blue)}.channel-grid{display:grid;grid-template-columns:repeat(3,1fr);gap:16px}.channel-heading{display:flex;align-items:center;justify-content:space-between}.table-wrap{overflow:auto}@media(max-width:760px){.channel-grid{grid-template-columns:1fr}}
</style>
