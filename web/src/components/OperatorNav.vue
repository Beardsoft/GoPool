<script setup lang="ts">
import { RouterLink } from 'vue-router'
</script>

<template>
  <nav class="operator-nav" data-operator-nav aria-label="Operator console">
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
  gap: 6px;
  padding: 8px;
  margin: 24px 0 32px;
  border: 1px solid var(--app-border);
  border-radius: 14px;
  background: var(--surface-1);
  box-shadow: var(--shadow-subtle);
  overflow-x: auto;
}
.operator-nav a {
  flex: 1;
  min-width: max-content;
  padding: 10px 18px;
  border-radius: 9px;
  color: var(--app-muted);
  text-decoration: none;
  font-weight: 600;
  text-align: center;
  transition: background .2s var(--nimiq-ease), color .2s var(--nimiq-ease);
}
.operator-nav a:hover { color: var(--app-text); background: var(--surface-2); }
.operator-nav a.router-link-active {
  color: white;
  background: var(--nimiq-light-blue-bg);
  box-shadow: 0 8px 20px rgba(5, 130, 202, .2);
}

@media (max-width: 640px) {
  .operator-nav { margin: 16px -8px 24px; border-radius: 12px; }
  .operator-nav a { padding: 9px 13px; font-size: .86rem; }
}
</style>
