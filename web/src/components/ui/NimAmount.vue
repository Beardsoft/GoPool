<script setup lang="ts">
import { computed } from 'vue'
import { formatNim } from '../../utils/format'

const props = withDefaults(defineProps<{
  luna: number
  showSymbol?: boolean
}>(), {
  showSymbol: true
})

const formatted = computed(() => formatNim(props.luna))
</script>

<template>
  <span class="nim-amount">
    {{ formatted }}<template v-if="props.showSymbol"> NIM</template>
  </span>
</template>

<style scoped>
.nim-amount {
  font-variant-numeric: tabular-nums;
}
</style>
