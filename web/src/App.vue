<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
import { onMounted, ref } from 'vue'

const dark = ref(false)
onMounted(() => {
  const saved = localStorage.getItem('gopool-dark')
  if (saved) dark.value = saved === '1'
  apply()
})
function apply() {
  document.documentElement.classList.toggle('dark', dark.value)
  localStorage.setItem('gopool-dark', dark.value ? '1' : '0')
}
function toggle() { dark.value = !dark.value; apply() }

const menuOpen = ref(false)
function closeMenu() { menuOpen.value = false }
</script>

<template>
  <div class="app-shell">
    <header class="nav">
      <div class="brand">
        <span class="logo">GoPool</span>
        <span class="tag">Nimiq Albatross</span>
      </div>
      <div class="nav-right">
        <nav class="nav-links" :class="{ open: menuOpen }">
          <RouterLink to="/" @click="closeMenu">Pool</RouterLink>
          <RouterLink to="/rewards" @click="closeMenu">Rewards</RouterLink>
          <RouterLink to="/epochs" @click="closeMenu">Epochs</RouterLink>
          <RouterLink to="/stakers" @click="closeMenu">Stakers</RouterLink>
          <RouterLink to="/onboard" @click="closeMenu">Onboard</RouterLink>
          <RouterLink to="/me" @click="closeMenu">My dashboard</RouterLink>
          <RouterLink to="/operator" @click="closeMenu">Operator</RouterLink>
        </nav>
        <button class="btn dark-toggle" @click="toggle" aria-label="Toggle dark mode">🌓</button>
        <button
          class="hamburger"
          :class="{ open: menuOpen }"
          :aria-expanded="menuOpen"
          aria-label="Toggle navigation menu"
          @click="menuOpen = !menuOpen"
        >
          <span class="bar"></span>
          <span class="bar"></span>
          <span class="bar"></span>
        </button>
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
.app-shell {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}
.nav {
  background: var(--nimiq-blue);
  color: white;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-16) var(--space-24);
  box-shadow: var(--shadow-sm);
  gap: var(--space-24);
  position: relative;
  z-index: 100;
}
.nav-right {
  display: flex;
  align-items: center;
  gap: var(--space-12);
}
.dark-toggle {
  padding: 6px 10px;
  font-size: 0.85rem;
}
.hamburger {
  display: none;
  flex-direction: column;
  justify-content: center;
  gap: 4px;
  width: 40px;
  height: 36px;
  padding: 8px;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  cursor: pointer;
}
.hamburger:hover {
  background: rgba(255, 255, 255, 0.08);
}
.hamburger .bar {
  display: block;
  width: 100%;
  height: 2px;
  background: white;
  border-radius: 2px;
  transition: transform 0.2s ease, opacity 0.2s ease;
}
.hamburger.open .bar:nth-child(1) {
  transform: translateY(6px) rotate(45deg);
}
.hamburger.open .bar:nth-child(2) {
  opacity: 0;
}
.hamburger.open .bar:nth-child(3) {
  transform: translateY(-6px) rotate(-45deg);
}
.brand {
  display: flex;
  align-items: baseline;
  gap: var(--space-12);
}
.logo {
  font-weight: 700;
  font-size: 1.25rem;
  letter-spacing: -0.01em;
}
.tag {
  font-size: 0.75rem;
  opacity: 0.8;
  text-transform: uppercase;
  letter-spacing: 0.08em;
}
.nav-links {
  display: flex;
  gap: var(--space-16);
  font-weight: 600;
}
.nav-links a {
  color: rgba(255,255,255,0.85);
  padding: 6px 10px;
  border-radius: var(--radius-sm);
  transition: background .2s ease, color .2s ease;
}
.nav-links a:hover {
  background: rgba(255,255,255,0.08);
  color: white;
  text-decoration: none;
}
.nav-links a.router-link-active {
  background: rgba(255,255,255,0.14);
  color: white;
}
.main {
  flex: 1;
  padding: var(--space-32) 0 var(--space-48);
}
@media (max-width: 720px) {
  .hamburger {
    display: inline-flex;
  }
  .nav-links {
    display: none;
    position: absolute;
    top: 100%;
    left: 0;
    right: 0;
    flex-direction: column;
    gap: 0;
    background: var(--nimiq-blue);
    border-top: 1px solid rgba(255, 255, 255, 0.1);
    box-shadow: var(--shadow-md);
    padding: var(--space-8) 0;
  }
  .nav-links.open {
    display: flex;
  }
  .nav-links a {
    padding: 12px var(--space-24);
    border-radius: 0;
  }
}
</style>
