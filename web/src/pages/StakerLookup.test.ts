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
vi.mock('../hub', () => ({ signStakingTransaction: vi.fn() }))

import StakerLookup from './StakerLookup.vue'
import { mockFetch } from '../test/helpers'
import { loadNetwork, resetExplorerForTests } from '../composables/useExplorer'
import { resetSessionCache } from '../composables/useSession'
import { signStakingTransaction } from '../hub'

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
    resetSessionCache()
    vi.mocked(signStakingTransaction).mockReset()
    mockFetch('/api/session', { error: 'not logged in' }, 401)
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

  it('shows a no-stake state (not an error) when the staker is unknown', async () => {
    mockFetch('/api/stakers/NQ00%20UNKNOWN%20STAKER%20ADDRESS%20HERE', { error: 'no staker' }, 404)
    mockFetch('/api/stakers/NQ00%20UNKNOWN%20STAKER%20ADDRESS%20HERE/history', { error: 'no staker' }, 404)
    const wrapper = await mountPage('NQ00 UNKNOWN STAKER ADDRESS HERE')
    const section = wrapper.get('[data-section="no-stake"]')
    expect(section.text()).toContain("This address isn't staking with us.")
    expect(section.get('button.btn').text()).toBe('Log in to stake')
  })

  it('shows the start-staking form for a signed-in staker with no stake', async () => {
    const MINE = 'NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E'
    mockFetch('/api/session', { address: MINE, operator: false })
    mockFetch(`/api/stakers/${encodeURIComponent(MINE)}`, { error: 'no staker' }, 404)
    mockFetch(`/api/stakers/${encodeURIComponent(MINE)}/history`, { error: 'no staker' }, 404)
    const wrapper = await mountPage()
    const section = wrapper.get('[data-section="no-stake"]')
    expect(section.text()).toContain('Delegate NIM to the pool')
    expect(section.get('#stake-amount').attributes('aria-label')).toBe('Amount in NIM')
    expect(section.get('button[type="submit"]').text()).toBe('Delegate to pool')
  })

  it('quotes, signs, and submits a stake, then shows the tx hash', async () => {
    const MINE = 'NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E'
    const TX = 'ee'.repeat(32)
    mockFetch('/api/pool', { network: 'test-albatross' })
    mockFetch('/api/session', { address: MINE, operator: false })
    mockFetch(`/api/stakers/${encodeURIComponent(MINE)}`, { error: 'no staker' }, 404)
    mockFetch(`/api/stakers/${encodeURIComponent(MINE)}/history`, { error: 'no staker' }, 404)
    mockFetch('/api/stake/quote', {
      tx: 'QUFBQQ==', amount_luna: 10_000_000, fee_luna: 1500,
      min_stake_luna: 10_000_000, balance_luna: 50_000_000,
      sender: MINE, delegate: MINE, validity_start_height: 12345,
    })
    mockFetch('/api/stake/submit', { tx_hash: TX })
    vi.mocked(signStakingTransaction).mockResolvedValue('ff'.repeat(32))
    await loadNetwork()

    const wrapper = await mountPage()
    const form = wrapper.get('[data-section="no-stake"] form')
    await form.get('#stake-amount').setValue('100')
    await form.trigger('submit')
    await flushPromises()
    await new Promise((r) => setTimeout(r, 0))

    expect(signStakingTransaction).toHaveBeenCalledWith(MINE, 'QUFBQQ==')
    expect(wrapper.text()).toContain('Your stake is on its way')
    expect(wrapper.html()).toContain(TX)
  })

  it('shows a "log in to manage" CTA linking to the dashboard', async () => {
    mockStakerApi()
    await loadNetwork()
    const wrapper = await mountPage(ADDR)
    const cta = wrapper.get('a.cta-manage')
    expect(cta.text()).toContain('Log in to manage your stake')
    expect(cta.attributes('href')).toBe('/me')
  })

  it('auto-loads the signed-in staker\'s own position with a Your stake badge', async () => {
    const MINE = 'NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E'
    mockFetch('/api/session', { address: MINE, operator: false })
    mockFetch(`/api/stakers/${encodeURIComponent(MINE)}`, {
      address: MINE, stake_luna: 100_000_000, percentage: 0.4335, payslips: [],
    })
    mockFetch(`/api/stakers/${encodeURIComponent(MINE)}/history`, {
      address: MINE, epochs: [], cumulative_reward_luna: 0,
    })
    const wrapper = await mountPage()
    expect(wrapper.text()).toContain('Your stake')
    expect(wrapper.text()).toContain('1,000 NIM')
  })
})
