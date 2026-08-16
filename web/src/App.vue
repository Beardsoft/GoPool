<script setup lang="ts">
import { onMounted } from 'vue'
import { RouterView } from 'vue-router'
import { loadNetwork } from './composables/useExplorer'

onMounted(() => { loadNetwork() })
</script>

<template>
  <RouterView />
</template>
