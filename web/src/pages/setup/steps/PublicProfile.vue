<script setup lang="ts">
import type { SetupDraft } from '../../../types/api'
const draft = defineModel<SetupDraft>('draft', { required: true })
</script>
<template><section><h2>Public profile</h2><label>Pool name<input v-model="draft.pool_name" name="pool_name" class="input" /></label><label>Description<textarea v-model="draft.pool_description" class="input" /></label><label>Contact URL<input v-model="draft.contact_url" class="input" type="url" /></label><label>Disclosure<textarea v-model="draft.disclosure" class="input" /></label></section></template>
