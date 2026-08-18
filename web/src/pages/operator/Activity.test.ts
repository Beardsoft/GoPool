import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import * as api from '../../api'
import Activity from './Activity.vue'

describe('Activity', () => {
  it('shows formatted payout context without expanding a details panel', async () => {
    vi.spyOn(api, 'apiGet').mockResolvedValue({
      items: [{
        id: 1,
        severity: 'info',
        category: 'payout',
        summary: 'Payout submitted',
        created_at: '2026-08-18T06:31:45Z',
        context_json: JSON.stringify({
          address: 'NQ95 HH5Q QT81 0VE5 V9SA LCNY CV37 K6Q6 XMPM',
          amount: 21584135,
          fee: 0,
          kind: 'delegate',
          txHash: '807c040f8be37948fb9bcb344158f42d543676a1e4b44a7effc25aee3df0593b',
        }),
      }],
      next_cursor: null,
      has_more: false,
    })
    const wrapper = mount(Activity)
    await flushPromises()
    expect(wrapper.find('details').exists()).toBe(false)
    expect(wrapper.text()).toContain('215.84135 NIM')
    expect(wrapper.text()).toContain('delegate')
    expect(wrapper.text()).toContain('807c040f…f0593b')
    expect(wrapper.find('[aria-label="Copy wallet address"]').exists()).toBe(true)
    expect(wrapper.find('[aria-label="Copy transaction hash"]').exists()).toBe(true)
  })
})
