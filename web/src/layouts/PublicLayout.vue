<script setup lang="ts">
import { computed, onMounted } from 'vue'
import AppHeader from '../components/AppHeader.vue'
import { loadNetwork, usePoolProfile } from '../composables/useExplorer'

const { contactUrl, telegramUrl, discordUrl, xUrl, disclosure } = usePoolProfile()
onMounted(() => { loadNetwork() })

const socials = computed(() => ([
  { key: 'contact', href: contactUrl.value, label: 'Contact operator' },
  { key: 'telegram', href: telegramUrl.value, label: 'Telegram' },
  { key: 'discord', href: discordUrl.value, label: 'Discord' },
  { key: 'x', href: xUrl.value, label: 'X' },
] as const).filter((item) => item.href))

const showFooter = computed(() => socials.value.length > 0 || Boolean(disclosure.value))
</script>

<template>
  <div class="public-layout">
    <AppHeader />
    <main class="main">
      <div class="container">
        <RouterView />
      </div>
    </main>
    <footer v-if="showFooter" class="public-footer">
      <div class="container footer-inner">
        <nav v-if="socials.length" class="footer-links" aria-label="Operator links">
          <a
            v-for="item in socials"
            :key="item.key"
            :data-profile="item.key"
            :href="item.href"
            rel="noopener noreferrer"
            target="_blank"
          >{{ item.label }}</a>
        </nav>
        <p v-if="disclosure" data-profile="disclosure">{{ disclosure }}</p>
      </div>
    </footer>
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
.public-footer {
  margin-top: auto;
  padding: 28px 0 36px;
  border-top: 1px solid var(--app-border);
  color: var(--app-muted);
}
.footer-inner {
  display: grid;
  gap: 10px;
  max-width: 1184px;
}
.footer-links {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 18px;
}
.public-footer a {
  color: var(--nimiq-light-blue);
  font-weight: 700;
  text-decoration: none;
}
.public-footer p {
  margin: 0;
  max-width: 72ch;
  line-height: 1.6;
  font-size: .9rem;
}
</style>
