<script setup lang="ts">
import AppHeader from '../components/AppHeader.vue'
import OperatorNav from '../components/OperatorNav.vue'
</script>

<template>
  <div class="operator-layout">
    <AppHeader />
    <div class="container">
      <OperatorNav />
      <RouterView />
    </div>
  </div>
</template>

<style scoped>
.operator-layout {
  min-height: 100vh;
}
.container {
  max-width: 960px;
  margin: 0 auto;
  padding: 0 var(--space-24);
}
</style>
