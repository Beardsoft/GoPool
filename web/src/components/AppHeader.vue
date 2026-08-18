<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useTheme } from '../composables/useTheme'
import { useSession } from '../composables/useSession'
import { loadNetwork, usePoolProfile } from '../composables/useExplorer'
import Identicon from './ui/Identicon.vue'
import { shortAddress } from '../utils/format'

const { theme, toggleTheme } = useTheme()
const { signedIn, address, operator, login, logout } = useSession()
const { poolName } = usePoolProfile()

const sessionMenuOpen = ref(false)
const sessionMenuRef = ref<HTMLElement | null>(null)

function onGlobalPointerDown(e: MouseEvent) {
  if (!sessionMenuOpen.value) return
  const root = sessionMenuRef.value
  if (root && !root.contains(e.target as Node)) sessionMenuOpen.value = false
}

function onGlobalKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') sessionMenuOpen.value = false
}

async function signOut() {
  sessionMenuOpen.value = false
  await logout()
}

onMounted(() => {
  loadNetwork()
  document.addEventListener('pointerdown', onGlobalPointerDown)
  document.addEventListener('keydown', onGlobalKeydown)
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onGlobalPointerDown)
  document.removeEventListener('keydown', onGlobalKeydown)
})
</script>

<template>
  <header class="app-header">
    <div class="container header-inner">
      <RouterLink to="/" class="brand" :aria-label="`${poolName} home`">
        <span class="brand-mark" aria-hidden="true"><span></span></span>
        <span class="brand-copy">
          <strong>{{ poolName }}</strong>
          <small>Nimiq staking</small>
        </span>
      </RouterLink>
      <div class="header-actions">
        <nav class="nav-links" aria-label="Primary navigation">
          <RouterLink to="/">Pool</RouterLink>
          <RouterLink to="/performance">Performance</RouterLink>
          <RouterLink to="/stakers">Find my stake</RouterLink>
          <RouterLink v-if="!signedIn || operator" to="/operator" class="operator-link">Operator</RouterLink>
        </nav>
        <button v-if="!signedIn" type="button" class="login-btn" @click="login">Sign in with Nimiq</button>
        <div v-else ref="sessionMenuRef" class="session-menu">
          <button
            type="button"
            class="session-chip"
            :aria-expanded="sessionMenuOpen"
            aria-haspopup="menu"
            :aria-label="`Account menu — ${address}`"
            @click="sessionMenuOpen = !sessionMenuOpen"
          >
            <Identicon :address="address" :size="22" />
            <span class="session-address" :title="address">{{ shortAddress(address) }}</span>
            <svg class="session-chevron" viewBox="0 0 12 12" fill="none" aria-hidden="true">
              <path d="M3 4.5 6 7.5 9 4.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
          </button>
          <div v-if="sessionMenuOpen" class="session-menu-panel" role="menu">
            <RouterLink to="/me" class="session-menu-item" role="menuitem" @click="sessionMenuOpen = false">My dashboard</RouterLink>
            <div class="session-menu-sep" />
            <button type="button" class="session-menu-item session-menu-item-danger" role="menuitem" @click="signOut">Sign out</button>
          </div>
        </div>
        <button
          type="button"
          class="theme-toggle"
          :aria-label="`Switch to ${theme === 'dark' ? 'light' : 'dark'} theme`"
          :title="`Use ${theme === 'dark' ? 'light' : 'dark'} theme`"
          @click="toggleTheme"
        >
          <svg v-if="theme === 'dark'" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>
          <svg v-else viewBox="0 0 24 24" aria-hidden="true"><path d="M20.4 15.4A8.5 8.5 0 0 1 8.6 3.6 8.5 8.5 0 1 0 20.4 15.4Z"/></svg>
        </button>
      </div>
    </div>
  </header>
</template>

<style scoped>
.app-header {
  position: sticky;
  top: 0;
  z-index: 20;
  color: white;
  background: color-mix(in srgb, var(--header-bg) 92%, transparent);
  border-bottom: 1px solid var(--header-border);
  backdrop-filter: blur(18px);
}
.header-inner {
  min-height: 76px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-24);
}
.brand {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  color: white;
  text-decoration: none;
}
.brand-mark {
  width: 38px;
  height: 42px;
  display: grid;
  place-items: center;
  background: var(--nimiq-gold-bg);
  clip-path: polygon(25% 6%, 75% 6%, 100% 50%, 75% 94%, 25% 94%, 0 50%);
  filter: drop-shadow(0 8px 16px rgba(233, 178, 19, .2));
}
.brand-mark span {
  width: 12px;
  height: 16px;
  border: 3px solid white;
  border-top: 0;
  border-radius: 0 0 8px 8px;
  transform: translateY(1px);
}
.brand-copy { display: grid; line-height: 1.05; }
.brand-copy strong { font-size: 1rem; letter-spacing: -.02em; }
.brand-copy small { margin-top: 5px; color: rgba(255,255,255,.58); font-size: .68rem; font-weight: 600; }
.header-actions { display: flex; align-items: center; gap: 18px; }
.nav-links {
  display: flex;
  align-items: center;
  gap: 4px;
}
.nav-links a {
  padding: 10px 13px;
  border-radius: 10px;
  color: rgba(255,255,255,.72);
  text-decoration: none;
  font-size: .9rem;
  font-weight: 600;
  transition: color .2s var(--nimiq-ease), background .2s var(--nimiq-ease);
}
.nav-links a:hover { color: white; background: rgba(255,255,255,.08); }
.nav-links a.router-link-active {
  color: white;
  background: rgba(255,255,255,.1);
}
.nav-links .operator-link { color: var(--nimiq-gold); }
.login-btn {
  padding: 10px 16px;
  border: 1px solid rgba(255, 255, 255, .22);
  border-radius: 10px;
  color: white;
  background: rgba(255, 255, 255, .08);
  font-size: .85rem;
  font-weight: 600;
  cursor: pointer;
}
.login-btn:hover { background: rgba(255, 255, 255, .15); }
.session-menu { position: relative; }
.session-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 5px 8px 5px 10px;
  border: 1px solid rgba(255, 255, 255, .14);
  border-radius: 999px;
  background: rgba(255, 255, 255, .08);
  cursor: pointer;
  font: inherit;
}
.session-chip:hover { background: rgba(255, 255, 255, .15); }
.session-address {
  color: white;
  font-family: var(--font-mono);
  font-size: .78rem;
  font-weight: 600;
}
.session-chevron {
  width: 12px;
  height: 12px;
  color: rgba(255, 255, 255, .6);
}
.session-menu-panel {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  z-index: 60;
  min-width: 200px;
  padding: 6px;
  border: 1px solid var(--app-border);
  border-radius: 12px;
  background: var(--surface-1);
  box-shadow: var(--shadow-elevated);
}
.session-menu-item {
  display: block;
  width: 100%;
  padding: 9px 12px;
  border: 0;
  border-radius: 8px;
  background: none;
  color: var(--app-text);
  font-size: .85rem;
  font-weight: 600;
  text-align: left;
  text-decoration: none;
  cursor: pointer;
}
.session-menu-item:hover { background: var(--bg-muted); }
.session-menu-item-danger { color: var(--danger-text); }
.session-menu-item-danger:hover { background: var(--danger-soft); }
.session-menu-sep {
  height: 1px;
  margin: 6px 4px;
  background: var(--app-border);
}
.theme-toggle {
  width: 40px;
  height: 40px;
  display: grid;
  place-items: center;
  border: 1px solid rgba(255,255,255,.14);
  border-radius: 50%;
  color: white;
  background: rgba(255,255,255,.08);
  cursor: pointer;
}
.theme-toggle:hover { background: rgba(255,255,255,.15); }
.theme-toggle svg { width: 19px; height: 19px; fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; }

@media (max-width: 760px) {
  .header-inner { min-height: 64px; }
  .brand-copy small { display: none; }
  .brand-mark { width: 30px; height: 34px; }
  .nav-links a:not(.operator-link):not(:first-child) { display: none; }
  .nav-links a { padding: 8px 9px; font-size: .82rem; }
  .header-actions { gap: 6px; }
}
</style>
