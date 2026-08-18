import { mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import { loadSettings, operatorTestGlobals } from '../../test/helpers'
import Settings from './Settings.vue'

it('shows before/after before saving settings', async () => {
  const wrapper = mount(Settings, { global: operatorTestGlobals })
  await loadSettings(wrapper)
  await wrapper.get('[name="min_payout_nim"]').setValue('25')
  await wrapper.get('[data-review]').trigger('click')
  expect(wrapper.get('[data-review-panel]').text()).toContain('10 NIM → 25 NIM')
})

it('lets the operator edit disclosure on the public profile', async () => {
  const wrapper = mount(Settings, { global: operatorTestGlobals })
  await loadSettings(wrapper)
  const disclosure = wrapper.get('[name="disclosure"]')
  expect(disclosure.element).toBeTruthy()
  await disclosure.setValue('Operated by Aurora Labs.')
  expect((disclosure.element as HTMLTextAreaElement).value).toBe('Operated by Aurora Labs.')
})
