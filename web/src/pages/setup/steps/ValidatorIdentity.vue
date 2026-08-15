<script setup lang="ts">
import type { SetupDraft } from '../../../types/api'
const draft = defineModel<SetupDraft>('draft', { required: true })
</script>
<template><section><h2>Validator identity</h2><p>The API stores only the public validator address. Mount the private key as the daemon-only secret documented in the deployment guide.</p><label>Validator address<input v-model="draft.validator_address" name="validator_address" class="input address" /></label><label>Fee wallet<input v-model="draft.pool_fee_wallet" name="pool_fee_wallet" class="input address" /></label><p class="muted">Key verification remains pending until the daemon heartbeat.</p></section></template>
