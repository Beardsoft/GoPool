<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { apiGet } from '../../api'
import type { OperatorActivityResponse, OperatorEvent } from '../../types/api'

const items = ref<OperatorEvent[]>([])
const nextCursor = ref<string | null>(null)
const hasMore = ref(false)
const severity = ref('')
const category = ref('')
const loading = ref(false)

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
  <div class="activity">
    <div class="filters">
      <select v-model="severity" @change="load()">
        <option value="">All severities</option>
        <option value="info">info</option>
        <option value="warning">warning</option>
        <option value="error">error</option>
      </select>
      <select v-model="category" @change="load()">
        <option value="">All categories</option>
      </select>
      <a :href="exportUrl()" target="_blank">Export</a>
    </div>

    <ul>
      <li v-for="e in items" :key="e.id">
        <strong>{{ e.severity }}</strong> {{ e.summary }}
        <details>
          <summary>Context</summary>
          <pre>{{ e.context_json }}</pre>
        </details>
      </li>
    </ul>

    <button v-if="hasMore" @click="load(nextCursor!)" :disabled="loading">Load more</button>
  </div>
</template>

<style scoped>
.filters { display: flex; gap: 12px; margin-bottom: 12px; }
.activity ul { list-style: none; padding: 0; }
.activity li { border-bottom: 1px solid var(--border); padding: 8px 0; }
</style>
