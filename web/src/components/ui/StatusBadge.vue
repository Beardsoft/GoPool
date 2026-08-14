<script setup lang="ts">
defineProps<{
  status: string
}>()
</script>

<template>
  <span class="status-badge" :data-status="status">{{ status }}</span>
</template>

<style scoped>
.status-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 0.75rem;
  font-weight: 600;
  background: var(--bg-muted);
  color: var(--text-80);
}
</style>
