import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MyDashboard from './MyDashboard.vue'

const ADDR = 'NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE'
const TX_DONE = 'aa'.repeat(32)
const TX_WAITING = 'bb'.repeat(32)

const meResponse = {
  address: ADDR,
  stake_luna: 1_000_000,
  percentage: 0.5,
  payslips: [
    { batch_number: 1, epoch_number: 1, amount_luna: 500, status: 'completed', tx_hash: TX_DONE },
    { batch_number: 2, epoch_number: 2, amount_luna: 700, status: 'awaiting_confirmation', tx_hash: TX_WAITING },
  ],
  transactions: [
    { hash: TX_DONE, status: 'completed', amount_luna: 500, submitted_at: '2026-01-01T00:00:00Z' },
    { hash: TX_WAITING, status: 'awaiting_confirmation', amount_luna: 700, submitted_at: new Date(Date.now() - 90 * 60_000).toISOString() },
  ],
  pending_luna: 700,
  paid_luna: 500,
  delegated: true,
  compound: true,
}

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

function mockMeApi() {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input.toString()
      if (url === '/api/me') return jsonResponse(meResponse)
      if (url === '/api/me/history') return jsonResponse({ cumulative_reward_luna: 1_200 })
      if (url === '/api/me/preference' && init?.method === 'PUT') {
        return jsonResponse({ compound: (JSON.parse(init.body as string) as { compound: boolean }).compound })
      }
      return new Response(JSON.stringify({ code: 'not_mocked', error: url }), { status: 500 })
    })
  )
}

async function mountPage() {
  const wrapper = mount(MyDashboard)
  await flushPromises()
  await new Promise((resolve) => setTimeout(resolve, 0))
  return wrapper
}

describe('MyDashboard profile', () => {
  it('renders pending/paid cards via formatNim', async () => {
    mockMeApi()
    const wrapper = await mountPage()
    const html = wrapper.html()
    expect(html).toContain('Awaiting payout')
    expect(html).toContain('Paid out')
    expect(html).toContain('0.007')
    expect(html).toContain('0.005')
  })

  it('shows the delegation badge', async () => {
    mockMeApi()
    const wrapper = await mountPage()
    expect(wrapper.html()).toContain('Delegated to pool')
  })

  it('shows elapsed time on pending rows', async () => {
    mockMeApi()
    const wrapper = await mountPage()
    expect(wrapper.html()).toContain('waiting 1h 30m')
  })

  it('fires PUT /api/me/preference with the new value on toggle', async () => {
    mockMeApi()
    const wrapper = await mountPage()
    const checkbox = wrapper.find('input[type="checkbox"]')
    await checkbox.setValue(false)
    await flushPromises()
    const fetchMock = vi.mocked(fetch)
    const putCall = fetchMock.mock.calls.find((c) => c[0] === '/api/me/preference')
    expect(putCall).toBeTruthy()
    const init = putCall![1]
    expect(init?.method).toBe('PUT')
    expect(JSON.parse(init!.body as string)).toEqual({ compound: false })
  })
})
