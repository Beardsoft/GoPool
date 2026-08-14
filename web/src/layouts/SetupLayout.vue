<script setup lang="ts">
</script>

<template>
  <div class="setup-layout">
    <header class="setup-header">
      <div class="container">
        <h1>GoPool Setup</h1>
      </div>
    </header>
    <main class="main">
      <div class="container">
        <RouterView />
      </div>
    </main>
  </div>
</template>

<style scoped>
.setup-layout {
  min-height: 100vh;
  background: var(--bg);
}
.setup-header {
  background: var(--nimiq-blue);
  color: white;
  padding: var(--space-16) 0;
}
.main {
  padding: var(--space-32) 0;
}
</style>
