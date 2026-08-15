import { flushPromises, mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import { mockAlertResponse } from '../../test/helpers'
import Alerts from './Alerts.vue'

it('never renders an alert token', async () => {
  mockAlertResponse({ telegram: { configured: true, enabled: true, destination_hint: 'chat …4821', state: 'configured' } })
  const wrapper = mount(Alerts)
  await flushPromises()
  expect(wrapper.text().toLowerCase()).not.toContain('secret')
  expect(wrapper.text()).toContain('chat …4821')
})
