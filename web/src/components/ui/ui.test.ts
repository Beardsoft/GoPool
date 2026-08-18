import { mount, flushPromises } from '@vue/test-utils'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import NimAmount from './NimAmount.vue'
import HoldConfirmButton from './HoldConfirmButton.vue'
import ExplorerLink from './ExplorerLink.vue'
import AddressIdentity from './AddressIdentity.vue'
import { mockFetch } from '../../test/helpers'
import { loadNetwork, resetExplorerForTests } from '../../composables/useExplorer'

describe('UI primitives', () => {
  beforeEach(() => {
    resetExplorerForTests()
  })

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

  it('shows an identicon next to account links', async () => {
    mockFetch('/api/pool', { network: 'test-albatross' })
    await loadNetwork()
    const wrapper = mount(ExplorerLink, {
      props: { kind: 'account', value: 'NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE' },
    })
    await flushPromises()
    expect(wrapper.find('img.identicon').exists()).toBe(true)
  })

  it('shows no identicon for transaction links', async () => {
    mockFetch('/api/pool', { network: 'test-albatross' })
    await loadNetwork()
    const wrapper = mount(ExplorerLink, { props: { kind: 'transaction', value: 'ab'.repeat(32) } })
    await flushPromises()
    expect(wrapper.find('img.identicon').exists()).toBe(false)
  })

  it('shows an identicon in AddressIdentity', async () => {
    const wrapper = mount(AddressIdentity, { props: { address: 'NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE' } })
    await flushPromises()
    expect(wrapper.find('img.identicon').exists()).toBe(true)
  })

  it('copies wallet addresses and transaction hashes', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } })
    const address = 'NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE'
    const hash = 'ab'.repeat(32)
    const account = mount(ExplorerLink, { props: { kind: 'account', value: address, copyable: true } })
    await account.get('[aria-label="Copy wallet address"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith(address)
    const tx = mount(ExplorerLink, { props: { kind: 'transaction', value: hash, copyable: true } })
    await tx.get('[aria-label="Copy transaction hash"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith(hash)
    expect(tx.get('button').attributes('aria-label')).toBe('Transaction hash copied')
  })
})
