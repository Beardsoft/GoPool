import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import * as api from '../../api'
import HoldConfirmButton from '../../components/ui/HoldConfirmButton.vue'
import Operations from './Operations.vue'

describe('Operations', () => {
  it('requires hold confirmation before posting retire', async () => {
    vi.spyOn(api, 'apiGet').mockResolvedValue({ items: [], next_cursor: 0 })
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
