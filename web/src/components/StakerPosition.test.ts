import { mount } from '@vue/test-utils'
import { describe, it, expect } from 'vitest'
import StakerPosition from './StakerPosition.vue'
import { positionFixture, historyFixture } from '../test/helpers'

describe('StakerPosition', () => {
  it('shows stake, rewards and export', () => {
    const wrapper = mount(StakerPosition, {
      props: {
        position: positionFixture,
        history: historyFixture,
        exportUrl: '/api/me/payslips.csv'
      }
    })
    expect(wrapper.text()).toContain('5 NIM')
    expect(wrapper.text()).toContain('2.5 NIM')
    expect(wrapper.get('a[download]').attributes('href')).toContain('payslips.csv')
  })
})
