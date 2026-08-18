<script setup lang="ts">
import { onMounted } from 'vue'
import AppHeader from '../components/AppHeader.vue'
import { loadNetwork, usePoolProfile } from '../composables/useExplorer'

const { contactUrl, disclosure } = usePoolProfile()
onMounted(() => { loadNetwork() })
</script>

<template>
  <div class="public-layout">
    <AppHeader />
    <main class="main">
      <div class="container">
        <RouterView />
      </div>
    </main>
    <footer v-if="contactUrl || disclosure" class="public-footer">
      <div class="container footer-inner">
        <a
          v-if="contactUrl"
          data-profile="contact"
          :href="contactUrl"
          rel="noopener noreferrer"
          target="_blank"
        >Contact operator</a>
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
