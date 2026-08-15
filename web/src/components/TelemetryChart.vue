<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import Chart from 'chart.js/auto'
import type { TelemetryPoint } from '../types/api'

const props = defineProps<{
  points: TelemetryPoint[]
  metric?: string
}>()

const canvasRef = ref<HTMLCanvasElement | null>(null)
let chart: Chart | null = null

function render() {
  if (!canvasRef.value) return
  const ctx = canvasRef.value.getContext('2d')
  if (!ctx) return
  if (chart) chart.destroy()
  chart = new Chart(ctx, {
    type: 'line',
    data: {
      labels: props.points.map(p => p.ts),
      datasets: [{
        label: props.metric ?? 'metric',
        data: props.points.map(p => p.value),
        tension: 0.3
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false
    }
  })
}

onMounted(render)
watch(() => props.points, render, { deep: true })
</script>

<template>
  <div data-section="telemetry" class="telemetry-chart">
    <canvas ref="canvasRef"></canvas>
  </div>
</template>

<style scoped>
.telemetry-chart { height: 200px; }
canvas { width: 100%; height: 100%; }
</style>
