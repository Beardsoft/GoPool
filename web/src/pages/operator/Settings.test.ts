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
