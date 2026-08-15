import { flushPromises, mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import { goToEconomics, mockSetupValidation, setupTestGlobals } from '../../test/helpers'
import Setup from './Setup.vue'

it('cannot advance economics with an invalid fee', async () => {
  mockSetupValidation({ field_errors: { pool_fee_percentage: 'Must be below 100%' } })
  const wrapper = mount(Setup, { global: setupTestGlobals })
  await goToEconomics(wrapper)
  await wrapper.get('[name="pool_fee_percentage"]').setValue('100')
  await wrapper.get('[data-next]').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('Must be below 100%')
  expect(wrapper.attributes('data-step')).toBe('economics')
})
