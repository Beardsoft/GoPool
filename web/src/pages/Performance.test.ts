import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { expect, it, vi } from 'vitest'
import Performance from './Performance.vue'

vi.mock('../components/RewardChart.vue', () => ({
  default: { template: '<div data-chart />', props: ['points', 'range'] },
}))

function stubPoolApis(points: unknown[], pool: Record<string, unknown> = {
  current_epoch: 24,
  epoch_status: 'in_progress',
  num_stakers: 3,
  total_stake_luna: 10_100_000_000,
  total_rewards_luna: 1_000_000,
  pool_fee_percentage: 0.01,
}) {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo) => {
    const url = String(input)
    const body = url.includes('/api/pool/rewards') ? points : pool
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }))
}

async function mountPerformance() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/performance', component: Performance },
      { path: '/epochs/:number', component: { template: '<div />' } },
    ],
  })
  await router.push('/performance')
  await router.isReady()
  const wrapper = mount(Performance, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

it('lists epochs newest first while keeping the last-20 window', async () => {
  const points = Array.from({ length: 25 }, (_, i) => ({
    epoch_number: i,
    total_amount: (i + 1) * 1000,
    total_fee: 10,
    batches: 1,
  }))
  stubPoolApis(points)

  const wrapper = await mountPerformance()

  const epochs = wrapper.findAll('tbody tr td:first-child').map((cell) => cell.text())
  expect(epochs[0]).toBe('24')
  expect(epochs[epochs.length - 1]).toBe('5')
  expect(epochs).toHaveLength(20)
})

it('shows total stake alongside rewards and fees', async () => {
  stubPoolApis([{
    epoch_number: 1,
    total_amount: 100_000_000,
    total_fee: 1_000_000,
    batches: 1,
  }], {
    current_epoch: 1,
    epoch_status: 'in_progress',
    num_stakers: 2,
    total_stake_luna: 10_100_000_000,
    total_rewards_luna: 100_000_000,
    pool_fee_percentage: 0.01,
  })

  const wrapper = await mountPerformance()
  const labels = wrapper.findAll('.stat .label').map((el) => el.text())
  expect(labels).toEqual(['Epochs shown', 'Total stake', 'Stakers', 'Total rewards', 'Total fees'])
  expect(wrapper.text()).toContain('101,000 NIM')

  const stakers = wrapper.findAll('.stat').find((el) => el.find('.label').text() === 'Stakers')
  expect(stakers?.find('.value').text()).toBe('2')
})
