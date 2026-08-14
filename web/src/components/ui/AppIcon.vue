<script setup lang="ts">
defineProps<{
  name: string
  size?: number
}>()
</script>

<template>
  <svg
    class="app-icon"
    :width="size ?? 24"
    :height="size ?? 24"
    aria-hidden="true"
    focusable="false"
  >
    <use :href="`#${name}`" />
  </svg>
</template>

<style scoped>
.app-icon {
  display: inline-block;
  vertical-align: middle;
}
</style>
