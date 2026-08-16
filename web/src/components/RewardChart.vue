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
let chart: Chart | null = null
let themeObserver: MutationObserver | null = null

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
  const dark = document.documentElement.dataset.theme === 'dark'
  const palette = {
    text: dark ? 'rgba(255, 255, 255, 0.68)' : 'rgba(31, 35, 72, 0.68)',
    grid: dark ? 'rgba(255, 255, 255, 0.10)' : 'rgba(31, 35, 72, 0.10)',
    rewards: dark ? '#0CA6FE' : '#0582CA',
    cumulative: '#21BCA5',
    cumulativeFill: dark ? 'rgba(33, 188, 165, 0.14)' : 'rgba(33, 188, 165, 0.10)',
    stake: dark ? '#F7C948' : '#E9B213',
  }
  const labels = filteredPoints.value.map(p => `Epoch ${p.epoch_number}`)
  const data = filteredPoints.value.map(p => p.total_amount)
  let running = 0
  const cumulative = filteredPoints.value.map(p => (running += p.total_amount))
  const stake = filteredPoints.value.map(p => p.total_stake_luna ?? 0)

  chart = new Chart(chartRef.value, {
    type: 'line',
    data: {
      labels,
      datasets: [
        {
          label: 'Epoch rewards',
          data,
          tension: 0.3,
          borderColor: palette.rewards,
          backgroundColor: palette.rewards,
          pointBackgroundColor: palette.rewards,
          borderWidth: 2,
          yAxisID: 'y'
        },
        {
          label: 'Cumulative rewards',
          data: cumulative,
          tension: 0.3,
          borderColor: palette.cumulative,
          backgroundColor: palette.cumulativeFill,
          borderWidth: 2,
          pointRadius: 0,
          fill: 'origin',
          borderDash: [4, 4],
          yAxisID: 'y'
        },
        {
          label: 'Total stake',
          data: stake,
          tension: 0.3,
          borderColor: palette.stake,
          backgroundColor: palette.stake,
          pointBackgroundColor: palette.stake,
          borderWidth: 2,
          yAxisID: 'y2'
        }
      ]
    },
    options: {
      responsive: true,
      interaction: { mode: 'index', intersect: false },
      scales: {
        x: {
          ticks: { color: palette.text },
          grid: { color: palette.grid }
        },
        y: {
          beginAtZero: true,
          title: { display: true, text: 'NIM', color: palette.text },
          grid: { color: palette.grid },
          ticks: { color: palette.text, callback: (v) => formatNim(Number(v)) }
        },
        y2: {
          position: 'right',
          beginAtZero: true,
          title: { display: true, text: 'Stake (NIM)', color: palette.text },
          grid: { drawOnChartArea: false },
          ticks: { color: palette.text, callback: (v) => formatNim(Number(v)) }
        }
      },
      plugins: {
        legend: { labels: { color: palette.text, usePointStyle: true } },
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

onMounted(() => {
  render()
  themeObserver = new MutationObserver(render)
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
})
onUnmounted(() => {
  themeObserver?.disconnect()
  if (chart) chart.destroy()
})
watch(() => props.points, render, { deep: true })
</script>

<template>
  <div>
    <canvas ref="chartRef" :aria-label="ariaLabel"></canvas>
  </div>
</template>
