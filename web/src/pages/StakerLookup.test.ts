import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'

const { chartConfigs } = vi.hoisted(() => ({ chartConfigs: [] as unknown[] }))
vi.mock('chart.js/auto', () => ({
  default: class {
    constructor(_canvas: HTMLCanvasElement, config: unknown) { chartConfigs.push(config) }
    destroy() {}
  },
}))

import StakerLookup from './StakerLookup.vue'
import { mockFetch } from '../test/helpers'
import { loadNetwork, resetExplorerForTests } from '../composables/useExplorer'

const ADDR = 'NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE'
const TX1 = 'ab'.repeat(32)
const TX2 = 'cd'.repeat(32)

function mockStakerApi() {
  mockFetch('/api/pool', { network: 'test-albatross' })
  mockFetch(`/api/stakers/${encodeURIComponent(ADDR)}`, {
    address: ADDR,
    stake_luna: 100_000_000,
    percentage: 0.4335,
    payslips: [
      { batch_number: 12, epoch_number: 14, amount_luna: 6_219_169, status: 'awaiting_confirmation', tx_hash: TX1 },
      { batch_number: 11, epoch_number: 13, amount_luna: 6_170_507, status: 'completed', tx_hash: TX2 },
    ],
  })
  mockFetch(`/api/stakers/${encodeURIComponent(ADDR)}/history`, {
    address: ADDR,
    epochs: [
      { epoch_number: 14, stake_luna: 100_000_000, percentage: 0.4335, reward_luna: 6_219_169 },
      { epoch_number: 13, stake_luna: 100_000_000, percentage: 0.7336, reward_luna: 6_170_507 },
    ],
    cumulative_reward_luna: 12_389_676,
  })
}

async function mountPage(address?: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/stakers/:address?', component: StakerLookup, props: true }],
  })
  await router.push('/stakers')
  await router.isReady()
  const wrapper = mount(StakerLookup, {
    global: { plugins: [router] },
    props: { address },
  })
  await flushPromises()
  await new Promise((resolve) => setTimeout(resolve, 0))
  return wrapper
}

describe('StakerLookup', () => {
  beforeEach(() => {
    chartConfigs.length = 0
    resetExplorerForTests()
  })

  it('shows the lookup form when no address is given', async () => {
    const wrapper = await mountPage()
    const section = wrapper.get('[data-section="staker-lookup"]')
    expect(section.get('#stake-address').attributes('aria-label')).toBe('Nimiq address')
    expect(section.text()).toContain('Find your stake')
  })

  it('shows formatted position, epoch rewards, and payout tx links', async () => {
    mockStakerApi()
    await loadNetwork()
    const wrapper = await mountPage(ADDR)
    const text = wrapper.text()

    expect(text).toContain('1,000 NIM')
    expect(text).toContain('123.89676 NIM')
    expect(text).toContain('62.19169 NIM')
    expect(text).toContain('61.70507 NIM')
    expect(text).toContain('Awaiting Confirmation')
    expect(text).toContain('Completed')

    const epochTable = wrapper.get('[data-section="epoch-history"] table')
    expect(epochTable.text()).toContain('1,000 NIM')

    const txLinks = wrapper.findAll('a[href*="/transaction/"]')
    expect(txLinks.map((a) => a.attributes('href'))).toEqual([
      `https://testnet.nimiqscan.com/transaction/${TX1}`,
      `https://testnet.nimiqscan.com/transaction/${TX2}`,
    ])

    const csv = wrapper.get('a[download]')
    expect(csv.attributes('href')).toBe(`/api/stakers/${encodeURIComponent(ADDR)}/payslips.csv`)

    const config = chartConfigs.at(-1) as { data: { datasets: { data: number[] }[] } }
    expect(config.data.datasets[0].data).toEqual([61.70507, 62.19169])
  })

  it('shows an error when the staker is unknown', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(
      JSON.stringify({ error: 'no staker with this address in this pool' }),
      { status: 404, headers: { 'Content-Type': 'application/json' } },
    )))
    const wrapper = await mountPage('NQ00 UNKNOWN STAKER ADDRESS HERE')
    expect(wrapper.get('[role="alert"]').text()).toContain('no staker')
  })

  it('shows a "log in to manage" CTA linking to the dashboard', async () => {
    mockStakerApi()
    await loadNetwork()
    const wrapper = await mountPage(ADDR)
    const cta = wrapper.get('a.cta-manage')
    expect(cta.text()).toContain('Log in to manage your stake')
    expect(cta.attributes('href')).toBe('/me')
  })
})
