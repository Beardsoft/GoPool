import { beforeEach, describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

const { chartConfigs } = vi.hoisted(() => ({ chartConfigs: [] as any[] }))
vi.mock('chart.js/auto', () => ({
  default: class {
    constructor(_canvas: HTMLCanvasElement, config: unknown) { chartConfigs.push(config) }
    destroy() {}
  }
}))
import RewardChart from './RewardChart.vue'

describe('RewardChart', () => {
  beforeEach(() => {
    chartConfigs.length = 0
    document.documentElement.dataset.theme = 'light'
  })

  it('summarizes plotted rewards', () => {
    const wrapper = mount(RewardChart, { props: { points: [{ epoch_number: 8, total_amount: 250_000, total_fee: 5_000, batches: 2 }], range: '20e' } })
    expect(wrapper.get('canvas').attributes('aria-label')).toContain('Epoch 8')
  })

  it('uses high-contrast Nimiq colors in dark mode', () => {
    document.documentElement.dataset.theme = 'dark'
    mount(RewardChart, { props: { points: [{ epoch_number: 8, total_amount: 250_000, total_fee: 5_000, batches: 2 }], range: '20e' } })

    const config = chartConfigs.at(-1)
    expect(config.data.datasets.map((dataset: any) => dataset.borderColor ?? dataset.backgroundColor)).toEqual([
      '#0CA6FE',
      '#21BCA5',
    ])
    expect(config.options.scales.y.ticks.color).toBe('rgba(255, 255, 255, 0.68)')
    expect(config.options.plugins.legend.labels.color).toBe('rgba(255, 255, 255, 0.68)')
  })
})
