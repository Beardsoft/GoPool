<script setup lang="ts">
import type { SetupDraft } from '../../../types/api'
defineProps<{ draft: SetupDraft }>()
</script>
<template><section><h2>Review and launch</h2><dl><dt>Pool</dt><dd>{{ draft.pool_name }}</dd><dt>Network</dt><dd>{{ draft.network }}</dd><dt>Fee</dt><dd>{{ (draft.pool_fee_percentage * 100).toFixed(2) }}%</dd><dt>Minimum payout</dt><dd>{{ draft.min_payout_luna / 100_000 }} NIM</dd><dt>Validator</dt><dd class="address">{{ draft.validator_address }}</dd></dl><p>No secret values are included. Completion writes the revision; the pool activates in-process once the daemon heartbeat matches.</p></section></template>
