import { vi } from 'vitest'
import type { StakerPosition } from '../types/api'

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

export const historyFixture = [
  { epoch_number: 7, reward_luna: 250_000 },
  { epoch_number: 8, reward_luna: 260_000 }
]
