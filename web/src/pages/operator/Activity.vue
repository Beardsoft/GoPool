<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { apiGet } from '../../api'
import type { OperatorActivityResponse, OperatorEvent } from '../../types/api'

const items = ref<OperatorEvent[]>([])
const nextCursor = ref<string | null>(null)
const hasMore = ref(false)
const severity = ref('')
const category = ref('')
const loading = ref(false)

const categories = computed(() => {
  const set = new Set<string>()
  items.value.forEach(i => { if (i.category) set.add(i.category) })
  return Array.from(set).sort()
})

function formatContext(e: OperatorEvent): string {
  if (!e.context_json) return ''
  try {
    const parsed = JSON.parse(e.context_json)
    return JSON.stringify(parsed, null, 2)
  } catch {
    return e.context_json
  }
}

async function load(cursor?: string) {
  loading.value = true
  const params = new URLSearchParams()
  if (severity.value) params.set('severity', severity.value)
  if (category.value) params.set('category', category.value)
  if (cursor) params.set('cursor', cursor)
  params.set('limit', '50')
  const url = `/api/operator/activity?${params.toString()}`
  const res = await apiGet<OperatorActivityResponse>(url)
  if (cursor) {
    items.value.push(...res.items)
  } else {
    items.value = res.items
  }
  nextCursor.value = res.next_cursor
  hasMore.value = res.has_more
  loading.value = false
}

function exportUrl() {
  const params = new URLSearchParams()
  if (severity.value) params.set('severity', severity.value)
  if (category.value) params.set('category', category.value)
  return `/api/operator/activity/export?${params.toString()}`
}

onMounted(() => load())
</script>

<template>
  <div class="activity-page">
    <header class="page-head">
      <div>
        <p class="eyebrow">Operator logs</p>
        <h1>Activity feed</h1>
        <p class="muted">Structured events from the operator daemon, validator and payout pipeline.</p>
      </div>
      <a :href="exportUrl()" target="_blank" class="btn">Export CSV</a>
    </header>

    <section class="card">
      <div class="filters">
        <select v-model="severity" @change="load()" class="input">
          <option value="">All severities</option>
          <option value="info">info</option>
          <option value="warning">warning</option>
          <option value="error">error</option>
        </select>
        <select v-model="category" @change="load()" class="input">
          <option value="">All categories</option>
          <option v-for="c in categories" :key="c" :value="c">{{ c }}</option>
        </select>
        <span class="count">{{ items.length }} events</span>
      </div>

      <ul class="event-list" v-if="items.length">
        <li v-for="e in items" :key="e.id" class="event-item">
          <span class="activity-dot" :data-severity="e.severity"></span>
          <div class="event-body">
            <div class="event-head">
              <strong>{{ e.summary }}</strong>
              <span class="meta">{{ e.category }}<template v-if="e.created_at"> · {{ new Date(e.created_at).toLocaleString() }}</template></span>
            </div>
            <details class="context" v-if="formatContext(e)">
              <summary>Context</summary>
              <pre>{{ formatContext(e) }}</pre>
            </details>
            <p v-else class="empty-context">No structured context recorded for this event.</p>
          </div>
        </li>
      </ul>
      <div v-else class="empty-state">
        <p>No activity matches the current filters.</p>
      </div>

      <button v-if="hasMore" class="btn secondary" @click="load(nextCursor!)" :disabled="loading">Load more</button>
    </section>
  </div>
</template>

<style scoped>
.activity-page { display: grid; gap: 24px; }
.page-head { display: grid; gap: 4px; }
.eyebrow { text-transform: uppercase; letter-spacing: .08em; font-size: .75rem; font-weight: 700; color: var(--nimiq-light-blue); }
.page-head h1 { margin: 0; }
.page-head .muted { margin: 4px 0 0; }

.filters { display: flex; gap: 12px; align-items: center; flex-wrap: wrap; margin-bottom: 20px; }
.filters .input { max-width: 260px; }
.filters .count { margin-left: auto; color: var(--app-faint); font-size: .85rem; font-weight: 700; }

.event-list { list-style: none; margin: 0; padding: 0; display: grid; gap: 0; }
.event-item { display: flex; gap: 14px; padding: 18px 0; border-top: 1px solid var(--app-border); }
.event-body { display: grid; gap: 8px; min-width: 0; flex: 1; }
.event-head { display: flex; flex-wrap: wrap; gap: 10px; align-items: baseline; }
.event-head strong { font-size: 1rem; }
.meta { color: var(--app-faint); font-size: .82rem; font-weight: 600; }

.activity-dot { width: 9px; height: 9px; margin-top: 7px; flex: 0 0 auto; border-radius: 50%; background: var(--nimiq-gold); }
.activity-dot[data-severity='error'] { background: var(--nimiq-red); }
.activity-dot[data-severity='info'] { background: var(--nimiq-light-blue); }
.activity-dot[data-severity='warning'] { background: var(--nimiq-gold); }

.context { margin-top: 4px; }
.context summary { cursor: pointer; color: var(--nimiq-light-blue); font-weight: 700; font-size: .86rem; }
.context pre { margin: 8px 0 0; padding: 12px; border-radius: 10px; background: var(--surface-2); color: var(--app-text); font-family: var(--font-mono); font-size: .78rem; overflow: auto; white-space: pre-wrap; word-break: break-word; }

.empty-context { margin: 0; color: var(--app-faint); font-size: .86rem; }
.empty-state { padding: 48px 0; text-align: center; color: var(--app-faint); }

.btn.secondary { background: var(--surface-1); color: var(--app-text); border: 1px solid var(--app-border); }
.btn.secondary:disabled { opacity: .55; cursor: not-allowed; }

@media (max-width: 640px) {
  .filters { flex-direction: column; align-items: stretch; }
  .filters .input { max-width: none; }
  .filters .count { margin-left: 0; }
}
</style>
