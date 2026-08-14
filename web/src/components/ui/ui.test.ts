import { mount } from '@vue/test-utils'
import { describe, it, expect, vi } from 'vitest'
import NimAmount from './NimAmount.vue'
import HoldConfirmButton from './HoldConfirmButton.vue'

describe('UI primitives', () => {
  it('renders luna as NIM', () => {
    const wrapper = mount(NimAmount, { props: { luna: 123_456_789 } })
    expect(wrapper.text()).toContain('1,234.56789 NIM')
  })

  it('does not confirm before 2 seconds', async () => {
    vi.useFakeTimers()
    const wrapper = mount(HoldConfirmButton)
    await wrapper.trigger('pointerdown')
    vi.advanceTimersByTime(1_999)
    expect(wrapper.emitted('confirm')).toBeFalsy()
    vi.advanceTimersByTime(1)
    expect(wrapper.emitted('confirm')).toHaveLength(1)
    vi.useRealTimers()
  })
})
