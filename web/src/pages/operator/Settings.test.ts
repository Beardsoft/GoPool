import { flushPromises, mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import { loadSettings, mockFetch, operatorTestGlobals, positionFixture } from '../../test/helpers'
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

it('shows Activating instead of Restart required while hashes differ', async () => {
  const wrapper = mount(Settings, { global: operatorTestGlobals })
  mockFetch('/api/operator/settings', {
    active_hash: 'abc', daemon_hash: 'old', restart_required: true,
    settings: {
      rpc_url: 'https://rpc-testnet.nimiqscan.com', network: 'test-albatross', pool_fee_wallet: positionFixture.address,
      pool_fee_percentage: 0.01, payout_mode: 'delegate', min_payout_luna: 1_000_000, auto_reactivate: true,
      api_addr: ':8080', validator_address: positionFixture.address, operator_addresses: '', metrics_addr: ':9100', pool_name: 'GoPool'
    }, secrets: { validator_key: 'configured', session_secret: 'configured', telegram_token: 'missing' }
  })
  await flushPromises()
  expect(wrapper.text()).not.toContain('Restart required')
  expect(wrapper.text()).toContain('Activating')
  wrapper.unmount()
})
