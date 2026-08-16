<script setup lang="ts">
import { computed } from 'vue'
import { useExplorer } from '../../composables/useExplorer'
import Identicon from './Identicon.vue'

const props = defineProps<{
  kind: 'account' | 'transaction'
  value: string
  label?: string
  title?: string
}>()

const { txUrl, accountUrl } = useExplorer()
const href = computed(() => (props.kind === 'transaction' ? txUrl(props.value) : accountUrl(props.value)))

const isHash = computed(() => /^[0-9a-fA-F]{64}$/.test(props.value))
const display = computed(() => (isHash.value ? `${props.value.slice(0, 10)}…${props.value.slice(-8)}` : props.value))
</script>

<template>
  <span class="explorer-link">
    <Identicon v-if="kind === 'account'" :address="value" />
    <a v-if="href" :href="href" target="_blank" rel="noopener noreferrer" :title="title ?? value">{{ label ?? value }}</a>
    <span v-else class="explorer-mono" :title="title ?? value">{{ display }}</span>
  </span>
</template>

<style scoped>
.explorer-link {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
a { color: var(--nimiq-light-blue); text-decoration: none; }
a:hover { text-decoration: underline; }
.explorer-mono {
  font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 0.85em;
  color: var(--text-60);
}
</style>
