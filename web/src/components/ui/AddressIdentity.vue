<script setup lang="ts">
import { computed } from 'vue'
import { shortAddress } from '../../utils/format'

const props = defineProps<{
  address: string
  compact?: boolean
  copyable?: boolean
}>()

const display = computed(() => {
  if (props.compact) return shortAddress(props.address)
  return props.address
})

const copy = async () => {
  try {
    await navigator.clipboard.writeText(props.address)
  } catch {}
}
</script>

<template>
  <span class="address-identity">
    <span class="address" :title="address">{{ display }}</span>
    <button v-if="copyable" class="copy-btn" @click="copy" type="button" aria-label="Copy address">Copy</button>
  </span>
</template>

<style scoped>
.address-identity {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-family: var(--font-mono);
}
.address {
  font-family: var(--font-mono);
}
.copy-btn {
  font-size: 0.75rem;
  padding: 2px 6px;
  border: 1px solid var(--border);
  background: var(--bg-elev);
  border-radius: var(--radius-sm);
  cursor: pointer;
}
.copy-btn:hover {
  background: var(--bg-muted);
}
</style>
