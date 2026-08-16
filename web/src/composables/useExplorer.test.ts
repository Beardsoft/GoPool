import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { useExplorer, loadNetwork, resetExplorerForTests } from './useExplorer'

function harness() {
  let api: ReturnType<typeof useExplorer>
  const wrapper = mount({
    template: '<div />',
    setup() {
      api = useExplorer()
      return {}
    },
  })
  return { api: () => api!, wrapper }
}

describe('useExplorer', () => {
  beforeEach(() => {
    resetExplorerForTests()
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({
      ok: true,
      json: () => Promise.resolve({ network: 'test-albatross' }),
    })))
  })

  it('maps test network to testnet explorer', async () => {
    const { api, wrapper } = harness()
    await loadNetwork()
    expect(api().txUrl('7d092803bb134384fea3847673369ac15df4854c37e9e7f03e590e6f4716ced2'))
      .toBe('https://testnet.nimiqscan.com/transaction/7d092803bb134384fea3847673369ac15df4854c37e9e7f03e590e6f4716ced2')
    expect(api().accountUrl('NQ87 T9QB 9RR6 5A4F EV3U 320T 2XK9 QGYS 2MJA'))
      .toBe('https://testnet.nimiqscan.com/account/NQ87%20T9QB%209RR6%205A4F%20EV3U%20320T%202XK9%20QGYS%202MJA')
    wrapper.unmount()
  })

  it('maps main network to mainnet explorer', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({
      ok: true,
      json: () => Promise.resolve({ network: 'main-albatross' }),
    })))
    const { api, wrapper } = harness()
    await loadNetwork()
    expect(api().txUrl('abc123')).toBe('https://nimiqscan.com/transaction/abc123')
    wrapper.unmount()
  })

  it('returns null urls on dev network', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({
      ok: true,
      json: () => Promise.resolve({ network: 'dev-albatross' }),
    })))
    const { api, wrapper } = harness()
    await loadNetwork()
    expect(api().txUrl('abc123')).toBeNull()
    expect(api().accountUrl('NQ87 T9QB')).toBeNull()
    wrapper.unmount()
  })

  it('detects tx hashes and nimiq addresses in free text', async () => {
    const { api, wrapper } = harness()
    await loadNetwork()
    expect(api().explorerUrlFor('7d092803bb134384fea3847673369ac15df4854c37e9e7f03e590e6f4716ced2'))
      .toEqual({ kind: 'transaction', url: 'https://testnet.nimiqscan.com/transaction/7d092803bb134384fea3847673369ac15df4854c37e9e7f03e590e6f4716ced2' })
    expect(api().explorerUrlFor('NQ87 T9QB 9RR6 5A4F EV3U 320T 2XK9 QGYS 2MJA'))
      .toEqual({ kind: 'account', url: 'https://testnet.nimiqscan.com/account/NQ87%20T9QB%209RR6%205A4F%20EV3U%20320T%202XK9%20QGYS%202MJA' })
    expect(api().explorerUrlFor('some error message')).toBeNull()
    wrapper.unmount()
  })
})
