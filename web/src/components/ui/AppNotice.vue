<script setup lang="ts">
defineProps<{
  type?: 'info' | 'warning' | 'error' | 'success'
}>()
</script>

<template>
  <div class="app-notice" :data-type="type ?? 'info'">
    <slot />
  </div>
</template>

<style scoped>
.app-notice {
  padding: var(--space-12) var(--space-16);
  border-radius: var(--radius-sm);
  background: var(--bg-muted);
  border: 1px solid var(--border);
  margin: var(--space-16) 0;
}
.app-notice[data-type='error'] {
  background: #fff1f0;
  border-color: #ffd0cd;
  color: var(--nimiq-red);
}
.app-notice[data-type='warning'] {
  background: #fff8e6;
  border-color: #ffe0a0;
}
.app-notice[data-type='success'] {
  background: #e8f7f0;
  border-color: #b7e6d5;
}
</style>
