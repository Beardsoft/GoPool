<script setup lang="ts">
import { onMounted, onUnmounted, ref, computed, watch } from 'vue'
import Chart from 'chart.js/auto'
import type { RewardPoint } from '../types/api'
import { formatNim } from '../utils/format'

const props = defineProps<{
  points: RewardPoint[]
  range: '20e' | '90d' | 'all'
}>()

const chartRef = ref<HTMLCanvasElement | null>(null)
const tableRef = ref<HTMLTableElement | null>(null)
let chart: Chart | null = null

const filteredPoints = computed(() => {
  // Simple range handling; for now just return points
  return props.points
})

const ariaLabel = computed(() => {
  if (!filteredPoints.value.length) return 'No reward data'
  const first = filteredPoints.value[0].epoch_number
  const last = filteredPoints.value[filteredPoints.value.length - 1].epoch_number
  return `Rewards from Epoch ${first} to Epoch ${last}`
})

function render() {
  if (!chartRef.value) return
  if (chart) chart.destroy()
  const labels = filteredPoints.value.map(p => `Epoch ${p.epoch_number}`)
  const data = filteredPoints.value.map(p => p.total_amount)
  const fees = filteredPoints.value.map(p => p.total_fee)
  let running = 0
  const cumulative = filteredPoints.value.map(p => (running += p.total_amount))

  chart = new Chart(chartRef.value, {
    type: 'line',
    data: {
      labels,
      datasets: [
        {
          label: 'Total rewards (luna)',
          data,
          tension: 0.3,
          borderWidth: 2,
          yAxisID: 'y'
        },
        {
          label: 'Cumulative rewards (luna)',
          data: cumulative,
          tension: 0.3,
          borderWidth: 2,
          pointRadius: 0,
          fill: 'origin',
          borderDash: [4, 4],
          yAxisID: 'y'
        },
        {
          label: 'Pool fee (luna)',
          data: fees,
          type: 'bar',
          backgroundColor: 'rgba(100,100,100,0.3)',
          yAxisID: 'y1'
        }
      ]
    },
    options: {
      responsive: true,
      interaction: { mode: 'index', intersect: false },
      scales: {
        y: {
          beginAtZero: true,
          title: { display: true, text: 'Luna' },
          ticks: { callback: (v) => formatNim(Number(v)) }
        },
        y1: {
          beginAtZero: true,
          position: 'right',
          grid: { drawOnChartArea: false },
          ticks: { callback: (v) => formatNim(Number(v)) }
        }
      },
      plugins: {
        tooltip: {
          callbacks: {
            label: (ctx) => {
              const v = ctx.parsed.y as number | null
              return v != null ? `${ctx.dataset.label}: ${formatNim(v)} NIM` : `${ctx.dataset.label}`
            }
          }
        }
      }
    }
  })
}

onMounted(render)
onUnmounted(() => { if (chart) chart.destroy() })
watch(() => props.points, render, { deep: true })
</script>

<template>
  <div>
    <canvas ref="chartRef" :aria-label="ariaLabel"></canvas>
    <table ref="tableRef" aria-label="Reward data table">
      <thead>
        <tr><th>Epoch</th><th>Total</th><th>Fee</th><th>Batches</th></tr>
      </thead>
      <tbody>
        <tr v-for="p in filteredPoints" :key="p.epoch_number">
          <td>{{ p.epoch_number }}</td>
          <td>{{ formatNim(p.total_amount) }} NIM</td>
          <td>{{ formatNim(p.total_fee) }} NIM</td>
          <td>{{ p.batches }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
