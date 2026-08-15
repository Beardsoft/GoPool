<script setup lang="ts">
import AppHeader from '../components/AppHeader.vue'
</script>

<template>
  <div class="public-layout">
    <AppHeader />
    <main class="main">
      <div class="container">
        <RouterView />
      </div>
    </main>
  </div>
</template>

<style scoped>
.public-layout {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}
.main {
  flex: 1;
  padding: 0 0 72px;
}
.main > .container {
  max-width: 1184px;
}
</style>
