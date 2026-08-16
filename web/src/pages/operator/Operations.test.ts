import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import * as api from '../../api'
import HoldConfirmButton from '../../components/ui/HoldConfirmButton.vue'
import Operations from './Operations.vue'

describe('Operations', () => {
  it('shows waiting time, submitted height, and stuck badge on payout rows', async () => {
    const payout = {
      hash: '0xabc',
      address: 'NQ00 0000 0000 0000 0000 0000 0000 0001',
      amount: 100_000,
      status: 'awaiting_confirmation',
      submitted_at: new Date(Date.now() - 2 * 3_600_000).toISOString(),
      submitted_height: 540,
      stuck: true,
    }
    vi.spyOn(api, 'apiGet')
      .mockResolvedValueOnce({ items: [payout], next_cursor: 0 })
      .mockResolvedValueOnce({ items: [], next_cursor: 0 })
    const wrapper = mount(Operations)
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('stuck')
    expect(text).toContain('h540')
    expect(text).toContain('waiting 2h 0m')
  })

  it('requires hold confirmation before posting retire', async () => {
    vi.spyOn(api, 'apiGet').mockResolvedValueOnce({ items: [], next_cursor: 0 }).mockResolvedValueOnce({ items: [], next_cursor: 0 })
    const post = vi.spyOn(api, 'apiPost').mockResolvedValue({ id: 1, action: 'retire', state: 'requested' })
    const wrapper = mount(Operations)
    await flushPromises()
    await wrapper.get('[data-action="retire"]').trigger('click')
    expect(post).not.toHaveBeenCalled()
    wrapper.findComponent(HoldConfirmButton).vm.$emit('confirm')
    await flushPromises()
    expect(post).toHaveBeenCalledWith('/api/operator/actions', { action: 'retire' })
  })
})
