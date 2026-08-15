<script setup lang="ts">
import { computed } from 'vue'; import type { SetupDraft } from '../../../types/api'
const draft = defineModel<SetupDraft>('draft', { required: true })
const feePercent = computed({ get: () => draft.value.pool_fee_percentage * 100, set: value => { draft.value.pool_fee_percentage = Number(value) / 100 } })
const minNim = computed({ get: () => draft.value.min_payout_luna / 100_000, set: value => { draft.value.min_payout_luna = Math.round(Number(value) * 100_000) } })
</script>
<template><section><h2>Pool economics</h2><label>Pool fee (%)<input v-model="feePercent" name="pool_fee_percentage" class="input" type="number" min="0" max="99.99" step="0.01" /></label><label>Minimum payout (NIM)<input v-model="minNim" name="min_payout_nim" class="input" type="number" min="0.00001" step="0.00001" /></label><label>Payout mode<select v-model="draft.payout_mode" class="input"><option value="delegate">Delegate</option><option value="transfer">Transfer</option></select></label><label><input v-model="draft.auto_reactivate" type="checkbox" /> Automatically reactivate when safe</label></section></template>
