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

  it('shows the reward epoch range on each payout row', async () => {
    const payout = {
      hash: '0xabc',
      address: 'NQ00 0000 0000 0000 0000 0000 0000 0001',
      amount: 100_000,
      status: 'awaiting_confirmation',
      submitted_height: 540,
      stuck: false,
      epoch_from: 12,
      epoch_to: 13,
    }
    vi.spyOn(api, 'apiGet')
      .mockResolvedValueOnce({ items: [payout], next_cursor: 0 })
      .mockResolvedValueOnce({ items: [], next_cursor: 0 })
    const wrapper = mount(Operations)
    await flushPromises()
    expect(wrapper.get('thead').text()).toContain('Epoch')
    expect(wrapper.get('tbody tr').text()).toContain('12–13')
  })

  it('loads more payouts and filters rows by epoch', async () => {
    const payout = (hash: string, epochFrom: number, epochTo: number) => ({
      hash,
      address: 'NQ00 0000 0000 0000 0000 0000 0001',
      amount: 100_000,
      status: 'completed',
      submitted_height: 540,
      stuck: false,
      epoch_from: epochFrom,
      epoch_to: epochTo,
    })
    const get = vi.spyOn(api, 'apiGet')
      .mockResolvedValueOnce({ items: [payout('0xa', 12, 13)], next_cursor: 50, has_more: true })
      .mockResolvedValueOnce({ items: [], next_cursor: 0 })
      .mockResolvedValueOnce({ items: [payout('0xb', 14, 14)], next_cursor: 51, has_more: false })
    const wrapper = mount(Operations)
    await flushPromises()
    expect(wrapper.get('tbody').findAll('tr').length).toBe(1)
    await wrapper.get('.btn.secondary').trigger('click')
    await flushPromises()
    expect(get).toHaveBeenLastCalledWith(expect.stringContaining('cursor=50'))
    expect(wrapper.get('tbody').findAll('tr').length).toBe(2)
    await wrapper.get('[aria-label="Filter payout epoch"]').setValue('14')
    expect(wrapper.get('tbody').findAll('tr').length).toBe(1)
    expect(wrapper.get('tbody tr').text()).toContain('14')
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
