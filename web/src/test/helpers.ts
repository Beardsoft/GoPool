import { vi } from 'vitest'
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
