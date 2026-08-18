<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useExplorer } from '../../composables/useExplorer'
import Identicon from './Identicon.vue'

const props = defineProps<{
  kind: 'account' | 'transaction'
  value: string
  label?: string
  title?: string
  copyable?: boolean
}>()

const { txUrl, accountUrl } = useExplorer()
const href = computed(() => (props.kind === 'transaction' ? txUrl(props.value) : accountUrl(props.value)))

const isHash = computed(() => /^[0-9a-fA-F]{64}$/.test(props.value))
const display = computed(() => (isHash.value ? `${props.value.slice(0, 10)}…${props.value.slice(-8)}` : props.value))
const copyLabel = computed(() => props.kind === 'transaction' ? 'Copy transaction hash' : 'Copy wallet address')
const copiedLabel = computed(() => props.kind === 'transaction' ? 'Transaction hash copied' : 'Wallet address copied')

const copied = ref(false)
let copiedTimer: ReturnType<typeof setTimeout> | undefined

async function copy() {
  try {
    await navigator.clipboard.writeText(props.value)
    copied.value = true
    clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => { copied.value = false }, 1500)
  } catch {}
}

onBeforeUnmount(() => clearTimeout(copiedTimer))
</script>

<template>
  <span class="explorer-link">
    <Identicon v-if="kind === 'account'" :address="value" />
    <a v-if="href" :href="href" target="_blank" rel="noopener noreferrer" :title="title ?? value">{{ label ?? value }}</a>
    <span v-else class="explorer-mono" :title="title ?? value">{{ label ?? display }}</span>
    <button
      v-if="copyable"
      type="button"
      class="copy-btn"
      :data-copied="copied || undefined"
      :aria-label="copied ? copiedLabel : copyLabel"
      :title="copied ? copiedLabel : copyLabel"
      @click="copy"
    >
      <svg v-if="copied" viewBox="0 0 16 16" aria-hidden="true"><path d="m3.5 8.5 3 3 6-7" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>
      <svg v-else viewBox="0 0 16 16" aria-hidden="true"><rect x="5.5" y="3.5" width="7" height="9" rx="1.2" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M3.5 5.5v7.2A1.3 1.3 0 0 0 4.8 14h6.2" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>
    </button>
  </span>
</template>

<style scoped>
.explorer-link {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
a { min-width: 0; overflow: hidden; color: var(--nimiq-light-blue); text-decoration: none; text-overflow: ellipsis; }
a:hover { text-decoration: underline; }
.explorer-mono {
  min-width: 0;
  overflow: hidden;
  color: var(--text-60);
  font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 0.85em;
  text-overflow: ellipsis;
}
.copy-btn {
  width: 24px;
  height: 24px;
  display: inline-grid;
  place-items: center;
  flex: 0 0 auto;
  padding: 0;
  border: 0;
  border-radius: 6px;
  color: var(--app-faint);
  background: transparent;
  cursor: pointer;
}
.copy-btn svg { width: 14px; height: 14px; }
.copy-btn:hover { color: var(--app-text); background: color-mix(in srgb, var(--app-text) 8%, transparent); }
.copy-btn[data-copied] { color: var(--nimiq-green); }
.copy-btn:focus-visible { outline: 2px solid var(--nimiq-light-blue); outline-offset: 1px; }
</style>
