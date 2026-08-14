<script setup lang="ts">
import { RouterLink } from 'vue-router'
</script>

<template>
  <nav class="operator-nav">
    <RouterLink to="/operator">Overview</RouterLink>
    <RouterLink to="/operator/activity">Activity</RouterLink>
    <RouterLink to="/operator/operations">Operations</RouterLink>
    <RouterLink to="/operator/alerts">Alerts</RouterLink>
    <RouterLink to="/operator/settings">Settings</RouterLink>
  </nav>
</template>

<style scoped>
.operator-nav {
  display: flex;
  gap: var(--space-16);
  padding: var(--space-12) 0;
  border-bottom: 1px solid var(--border);
  margin-bottom: var(--space-24);
}
.operator-nav a {
  color: var(--text-80);
  text-decoration: none;
  font-weight: 600;
}
.operator-nav a.router-link-active {
  color: var(--nimiq-blue);
  border-bottom: 2px solid var(--nimiq-blue);
  padding-bottom: 4px;
}
</style>
