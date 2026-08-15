import { flushPromises, mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import OperatorOverview from './pages/operator/Overview.vue'
import { mockFetch, operatorTestGlobals, overviewFixture } from './test/helpers'

vi.stubGlobal('EventSource', vi.fn(() => ({ onopen: null, onmessage: null, onerror: null, close: vi.fn() })))

it.each(['light', 'dark'])('keeps status text and keyboard focus in %s', async theme => {
  document.documentElement.dataset.theme = theme
  mockFetch('/api/operator/overview', overviewFixture())
  mockFetch('/api/pool', {
    current_epoch: 2, epoch_status: 'in_progress', num_stakers: 1,
    total_stake_luna: 10_100_000_000, total_rewards_luna: 177_489_526,
    pool_fee_percentage: 0.01,
  })
  const wrapper = mount(OperatorOverview, {
    attachTo: document.body,
    global: {
      ...operatorTestGlobals,
      stubs: { RouterLink: { template: '<a href="#"><slot /></a>' } },
    },
  })
  await flushPromises()
  expect(wrapper.get('[role="status"]').text()).toMatch(/healthy|attention|offline/i)
  const first = wrapper.find('a,button,input')
  const firstElement = first.element as HTMLElement
  firstElement.focus()
  expect(document.activeElement).toBe(first.element)
  wrapper.unmount()
})
