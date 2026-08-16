<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: string
}>()

const label = computed(() =>
  props.status
    .replaceAll('_', ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
)

const tone = computed(() => {
  const s = props.status
  if (['completed', 'confirmed', 'executed', 'delivered', 'healthy', 'ok', 'configured'].includes(s)) return 'success'
  if (['failed', 'error', 'missing', 'invalid', 'unavailable'].includes(s)) return 'danger'
  if (['awaiting_confirmation', 'pending', 'submitted', 'processing', 'out_for_payment', 'requested', 'in_progress'].includes(s)) return 'pending'
  return 'neutral'
})
</script>

<template>
  <span class="status-badge" :data-status="status" :data-tone="tone">{{ label }}</span>
</template>

<style scoped>
.status-badge {
  display: inline-block;
  padding: 3px 10px;
  border-radius: 999px;
  font-size: 0.75rem;
  font-weight: 700;
  white-space: nowrap;
  background: var(--bg-muted);
  color: var(--text-80);
}
.status-badge[data-tone='success'] {
  background: var(--success-soft);
  color: var(--success-text);
}
.status-badge[data-tone='danger'] {
  background: var(--danger-soft);
  color: var(--danger-text);
}
.status-badge[data-tone='pending'] {
  background: color-mix(in srgb, var(--nimiq-light-blue) 12%, transparent);
  color: var(--nimiq-light-blue);
}
</style>
