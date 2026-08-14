<script setup lang="ts">
import { RouterLink } from 'vue-router'
</script>

<template>
  <header class="app-header">
    <div class="container header-inner">
      <RouterLink to="/" class="brand">GoPool</RouterLink>
      <nav class="nav-links">
        <RouterLink to="/">Pool</RouterLink>
        <RouterLink to="/epochs">Epochs</RouterLink>
        <RouterLink to="/stakers">Stakers</RouterLink>
        <RouterLink to="/operator">Operator</RouterLink>
      </nav>
    </div>
  </header>
</template>

<style scoped>
.app-header {
  background: var(--nimiq-blue);
  color: white;
  padding: var(--space-12) 0;
}
.header-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.brand {
  font-weight: 700;
  color: white;
  text-decoration: none;
}
.nav-links {
  display: flex;
  gap: var(--space-16);
}
.nav-links a {
  color: rgba(255,255,255,0.85);
  text-decoration: none;
}
.nav-links a.router-link-active {
  color: white;
  font-weight: 600;
}
</style>
