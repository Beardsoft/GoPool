import { vi } from 'vitest'
import type { StakerPosition, OperatorOverview } from '../types/api'
import { createRouter, createMemoryHistory } from 'vue-router'
import { flushPromises, type VueWrapper } from '@vue/test-utils'

const responses = new Map<string, { body: unknown; status: number }>()
export function mockFetch(path: string, body: unknown, status = 200) {
  responses.set(path, { body, status })
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString()
    const hit = responses.get(url)
    if (!hit) return new Response(JSON.stringify({ code: 'not_mocked', error: url }), { status: 500 })
    return new Response(JSON.stringify(hit.body), { status: hit.status, headers: { 'Content-Type': 'application/json' } })
  }))
}

export const positionFixture: StakerPosition = {
  address: 'NQ12 8D4K AAAA BBBB CCCC DDDD EEEE FFFF GGGG',
  stake_luna: 500_000,
  percentage: 2.5
}

export const historyFixture = {
  cumulative_reward_luna: 250_000
}

export function overviewFixture(overrides: Partial<OperatorOverview> = {}): OperatorOverview {
  return {
    status: 'ok',
    chain_lag: 0,
    metrics: {
      total_stake_luna: 1_000_000_000,
      total_rewards_luna: 500_000_000,
      num_stakers: 42,
      wallet_runway_days: 30
    },
    attention: [],
    validator: {
      address: 'NQ12 8D4K AAAA BBBB CCCC DDDD EEEE FFFF GGGG',
      state: 'active',
      last_processed_height: 123456
    },
    telemetry_points: [],
    recent_activity: [],
    ...overrides
  }
}

export const operatorTestGlobals = {
  plugins: [createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div />' } }] })]
}

export function mockAlertResponse(channels: Record<string, unknown>) {
  mockFetch('/api/operator/alerts', { channels })
  mockFetch('/api/operator/alerts/deliveries?limit=50', { items: [], next_cursor: 0 })
}

export const setupTestGlobals = {
  plugins: [createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div />' } }] })]
}

export function mockSetupValidation(response: unknown) {
	  let validations = 0
	  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
	    const url = typeof input === 'string' ? input : input.toString()
	    let body: unknown = { code: 'not_mocked', error: url }
	    let status = 200
	    if (url === '/api/setup/session') body = { expires_in: 1800 }
	    else if (url === '/api/setup/status') body = { configured: false, checks: {} }
	    else if (url === '/api/setup/validate') body = validations++ === 0 ? { valid: true, field_errors: {} } : response
	    else status = 500
	    return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
	  }))
}

export async function goToEconomics(wrapper: VueWrapper) {
  await wrapper.get('[name="setup_token"]').setValue('bootstrap')
  await wrapper.get('[data-next]').trigger('click')
  await flushPromises()
  await wrapper.get('[data-next]').trigger('click')
  await flushPromises()
}

export async function loadSettings(wrapper: VueWrapper) {
  mockFetch('/api/operator/settings', {
    active_hash: 'abc', daemon_hash: 'abc', restart_required: false,
    settings: {
      rpc_url: 'https://rpc-testnet.nimiqscan.com', network: 'test-albatross', pool_fee_wallet: positionFixture.address,
      pool_fee_percentage: 0.01, payout_mode: 'delegate', min_payout_luna: 1_000_000, auto_reactivate: true,
      api_addr: ':8080', validator_address: positionFixture.address, operator_addresses: '', metrics_addr: ':9100', pool_name: 'GoPool'
    }, secrets: { validator_key: 'configured', session_secret: 'configured', telegram_token: 'missing' }
  })
  await flushPromises()
}
