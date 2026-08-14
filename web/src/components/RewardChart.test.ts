import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import RewardChart from './RewardChart.vue'

describe('RewardChart', () => {
  it('summarizes plotted rewards', () => {
    const wrapper = mount(RewardChart, { props: { points: [{ epoch_number: 8, total_amount: 250_000, total_fee: 5_000, batches: 2 }], range: '20e' } })
    expect(wrapper.get('canvas').attributes('aria-label')).toContain('Epoch 8')
    expect(wrapper.text()).toContain('2.5 NIM')
  })
})
