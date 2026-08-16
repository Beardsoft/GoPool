<script setup lang="ts">
import { ref, watch } from 'vue'
import Identicons from '@nimiq/identicons'
import identiconsSvgUrl from '@nimiq/identicons/dist/identicons.min.svg?url'

// The browser bundle ships without the SVG template inlined and fetches
// Identicons.svgPath at runtime; point it at the asset Vite emits.
Identicons.svgPath = identiconsSvgUrl

const props = withDefaults(defineProps<{ address: string; size?: number }>(), { size: 20 })
const src = ref('')

watch(
  () => props.address,
  async (address) => {
    const value = address.trim()
    if (!/^N[QK][0-9A-Z]{2}( [0-9A-Z]{4}){8}$/.test(value)) {
      src.value = ''
      return
    }
    src.value = await Identicons.toDataUrl(value)
  },
  { immediate: true },
)
</script>

<template>
  <img v-if="src" class="identicon" :src="src" :width="size" :height="size" :alt="`Identicon for ${address}`" />
</template>

<style scoped>
.identicon {
  border-radius: 5px;
  flex-shrink: 0;
}
</style>
