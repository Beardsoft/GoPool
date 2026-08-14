<script setup lang="ts">
defineProps<{
  title?: string
  description?: string
}>()
</script>

<template>
  <div class="empty-state">
    <div class="empty-content">
      <slot name="icon" />
      <h3 v-if="title">{{ title }}</h3>
      <p v-if="description" class="muted">{{ description }}</p>
      <slot />
    </div>
  </div>
</template>

<style scoped>
.empty-state {
  padding: var(--space-32);
  text-align: center;
  border: 1px dashed var(--border);
  border-radius: var(--radius);
  background: var(--bg-elev);
}
.empty-content {
  max-width: 360px;
  margin: 0 auto;
}
.muted {
  color: var(--text-60);
}
</style>
