<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
</script>

<template>
  <header class="nav">
    <RouterLink to="/">Pool</RouterLink>
    <RouterLink to="/epochs">Epochs</RouterLink>
    <RouterLink to="/stakers">Look up a staker</RouterLink>
    <RouterLink to="/me">My dashboard</RouterLink>
    <RouterLink to="/operator">Operator</RouterLink>
  </header>
  <main>
    <RouterView />
  </main>
</template>

<style scoped>
.nav {
  display: flex;
  gap: var(--space-16, 16px);
  padding: var(--space-16, 16px);
  font-family: 'Muli', sans-serif;
}
</style>
