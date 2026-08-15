import { flushPromises, mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import OperatorOverview from './pages/operator/Overview.vue'
import { mockFetch, overviewFixture } from './test/helpers'

vi.stubGlobal('EventSource', vi.fn(() => ({ onopen: null, onmessage: null, onerror: null, close: vi.fn() })))

it.each(['light', 'dark'])('keeps status text and keyboard focus in %s', async theme => {
  document.documentElement.dataset.theme = theme
  mockFetch('/api/operator/overview', overviewFixture())
  const wrapper = mount(OperatorOverview, { attachTo: document.body })
  await flushPromises()
  expect(wrapper.get('[role="status"]').text()).toMatch(/healthy|attention|offline/i)
  const first = wrapper.find('a,button,input')
  const firstElement = first.element as HTMLElement
  firstElement.focus()
  expect(document.activeElement).toBe(first.element)
  wrapper.unmount()
})
