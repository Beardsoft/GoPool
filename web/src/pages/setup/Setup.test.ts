import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { expect, it, vi } from 'vitest'
import { goToEconomics, mockSetupValidation, setupTestGlobals } from '../../test/helpers'
import Setup from './Setup.vue'

const hintAddress = 'NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E'

async function mountSetup(path = '/setup') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/setup', component: Setup }],
  })
  await router.push(path)
  await router.isReady()
  return mount(Setup, { global: { plugins: [router] } })
}

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

function mockSetupLaunch(handlers: Record<string, { body: unknown; status?: number }>) {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString()
    if (url === '/api/setup/session') return json({ expires_in: 1800 })
    if (url === '/api/setup/status') return json({ configured: false, checks: {} })
    if (url === '/api/setup/validate') return json({ valid: true, field_errors: {} })
    if (url === '/api/setup/complete') return json({ hash: 'abc123' }, 201)
    const hit = handlers[url]
    if (hit) return json(hit.body, hit.status ?? 200)
    return json({ code: 'not_mocked', error: url }, 500)
  }))
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

async function completeSetup(wrapper: ReturnType<typeof mount>) {
  await wrapper.get('[name="setup_token"]').setValue('bootstrap')
  for (let i = 0; i < 6; i++) {
    await wrapper.get('[data-next]').trigger('click')
    await flushPromises()
  }
}

it('polls after launch instead of showing a compose restart', async () => {
  mockSetupLaunch({
    '/api/pool': { body: { current_epoch: 1, epoch_status: 'in_progress', num_stakers: 0, total_stake_luna: 0, total_rewards_luna: 0, pool_fee_percentage: 0.01 } },
  })
  const wrapper = mount(Setup, { global: setupTestGlobals })
  await completeSetup(wrapper)
  expect(wrapper.text().toLowerCase()).not.toContain('docker compose')
  await flushPromises()
  expect(wrapper.text()).toContain('Pool is live')
})

it('shows readiness_error text after launch', async () => {
  mockSetupLaunch({
    '/api/pool': { body: { current_epoch: 0 }, status: 200 },
    '/api/operator/readiness': { body: { rpc_ok: true, readiness_error: 'validator not found', validator_state: 'unready' } },
  })
  const wrapper = mount(Setup, { global: setupTestGlobals })
  await completeSetup(wrapper)
  await flushPromises()
  expect(wrapper.text()).toContain('validator not found')
})

it('applies status hints and skips the token step when the URL has a token', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString()
    if (url === '/api/setup/session') return json({ expires_in: 1800 })
    if (url === '/api/setup/status') {
      return json({
        configured: false,
        checks: {},
        hints: {
          validator_address: hintAddress,
          pool_fee_wallet: hintAddress,
          network: 'main-albatross',
          rpc_url: 'https://rpc-mainnet.nimiqscan.com',
        },
      })
    }
    return json({ code: 'not_mocked', error: url }, 500)
  }))
  const wrapper = await mountSetup('/setup?token=bootstrap')
  await flushPromises()
  expect(wrapper.attributes('data-step')).toBe('validator')
  expect((wrapper.get('[name="validator_address"]').element as HTMLInputElement).value).toBe(hintAddress)
  expect((wrapper.get('[name="pool_fee_wallet"]').element as HTMLInputElement).value).toBe(hintAddress)
})
