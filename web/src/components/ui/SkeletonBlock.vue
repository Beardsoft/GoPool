<script setup lang="ts">
defineProps<{
  width?: string
  height?: string
}>()
</script>

<template>
  <div class="skeleton-block" :style="{ width, height }" />
</template>

<style scoped>
.skeleton-block {
  background: linear-gradient(90deg, var(--bg-muted) 25%, var(--bg-elev) 50%, var(--bg-muted) 75%);
  background-size: 200% 100%;
  animation: skeleton 1.2s infinite;
  border-radius: var(--radius-sm);
}
@keyframes skeleton {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}
</style>
