<script setup lang="ts">
import { ref, onUnmounted } from 'vue'

const emit = defineEmits<{ (e: 'confirm'): void }>()

const holding = ref(false)
const progress = ref(0)
let timerId: ReturnType<typeof setTimeout> | undefined

const startHold = () => {
  holding.value = true
  progress.value = 0
  timerId = setTimeout(() => {
    holding.value = false
    progress.value = 100
    emit('confirm')
  }, 2000)
}

const cancelHold = () => {
  holding.value = false
  progress.value = 0
  if (timerId) {
    clearTimeout(timerId)
    timerId = undefined
  }
}

const onKeyDown = (e: KeyboardEvent) => {
  if (e.key === ' ' || e.key === 'Enter') {
    e.preventDefault()
    cancelHold()
    emit('confirm')
  }
}

onUnmounted(() => {
  cancelHold()
})
</script>

<template>
  <button
    class="hold-confirm"
    @pointerdown="startHold"
    @mousedown="startHold"
    @pointerup="cancelHold"
    @mouseup="cancelHold"
    @pointercancel="cancelHold"
    @pointerleave="cancelHold"
    @keydown="onKeyDown"
    aria-describedby="hold-desc"
  >
    Hold to confirm
    <span v-if="holding" class="progress">{{ progress }}%</span>
    <span id="hold-desc" class="sr-only">Hold for 2 seconds to confirm, or press Space/Enter</span>
  </button>
</template>

<style scoped>
.hold-confirm {
  padding: 10px 16px;
  border-radius: var(--radius-sm);
  background: var(--nimiq-red);
  color: white;
  border: none;
  font-weight: 600;
  cursor: pointer;
}
.progress {
  margin-left: 8px;
  font-weight: 500;
}
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0,0,0,0);
  border: 0;
}
</style>
