<script setup lang="ts">
defineProps<{ token: string; checks?: Record<string, unknown> }>()
defineEmits<{ (event: 'update:token', value: string): void }>()
</script>
<template><section><h2>System check</h2><p>Exchange the one-time deployment token. It is never stored in the browser configuration.</p><label>Setup token<input name="setup_token" class="input" type="password" :value="token" @input="$emit('update:token', ($event.target as HTMLInputElement).value)" /></label></section></template>
