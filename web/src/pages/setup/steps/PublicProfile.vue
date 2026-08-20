<script setup lang="ts">
import type { SetupDraft } from '../../../types/api'
const draft = defineModel<SetupDraft>('draft', { required: true })
</script>
<template>
  <section>
    <h2>Public profile</h2>
    <label>Pool name<input v-model="draft.pool_name" name="pool_name" class="input" /></label>
    <label>Description<textarea v-model="draft.pool_description" class="input" /></label>
    <label>Contact URL<input v-model="draft.contact_url" class="input" type="url" /></label>
    <label>Telegram URL<input v-model="draft.telegram_url" name="telegram_url" class="input" type="url" placeholder="https://t.me/yourpool" /></label>
    <label>Discord URL<input v-model="draft.discord_url" name="discord_url" class="input" type="url" placeholder="https://discord.gg/invite" /></label>
    <label>X URL<input v-model="draft.x_url" name="x_url" class="input" type="url" placeholder="https://x.com/yourpool" /></label>
    <label>Disclosure<textarea v-model="draft.disclosure" class="input" /></label>
  </section>
</template>
