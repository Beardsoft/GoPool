import { vi } from 'vitest'
import type { StakerPosition, OperatorOverview } from '../types/api'
import { createRouter, createMemoryHistory } from 'vue-router'

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
  router: createRouter({ history: createMemoryHistory(), routes: [] })
}
