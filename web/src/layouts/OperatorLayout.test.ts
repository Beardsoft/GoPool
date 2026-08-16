import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import OperatorLayout from './OperatorLayout.vue'
import Overview from '../pages/operator/Overview.vue'

const loginWithHub = vi.fn()

vi.mock('../hub', () => ({
  loginWithHub: () => loginWithHub(),
}))

const eventSource = vi.fn(() => ({
  onopen: null,
  onmessage: null,
  onerror: null,
  close: vi.fn(),
}))

function routerForOperator() {
  const placeholder = { template: '<div />' }
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: placeholder },
      { path: '/performance', component: placeholder },
      { path: '/stakers', component: placeholder },
      {
        path: '/operator',
        component: OperatorLayout,
        children: [
          { path: '', component: Overview },
          { path: 'activity', component: placeholder },
          { path: 'operations', component: placeholder },
          { path: 'alerts', component: placeholder },
          { path: 'settings', component: placeholder },
        ],
      },
    ],
  })
}

function overviewResponse() {
  return {
    status: 'healthy',
    chain_lag: 0,
    wallet_runway_days: 30,
    readiness: 'ok',
    payout_summary: {},
    validator_summary: {
      address: 'NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E',
      state: 'active',
      last_processed_height: 816,
      last_tick_ms: 42,
    },
    attention: [],
    events: [],
  }
}

async function mountOperator(status: number, body: unknown) {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => new Response(JSON.stringify(
    String(input) === '/api/pool'
      ? { current_epoch: 2, epoch_status: 'in_progress', num_stakers: 1, total_stake_luna: 10_100_000_000, total_rewards_luna: 177_489_526, pool_fee_percentage: 0.01 }
      : String(input).startsWith('/api/operator/telemetry?') ? [] : body,
  ), {
    status: String(input) === '/api/pool' || String(input).startsWith('/api/operator/telemetry?') ? 200 : status,
    headers: { 'Content-Type': 'application/json' },
  })))
  const router = routerForOperator()
  await router.push('/operator')
  await router.isReady()
  const wrapper = mount(defineComponent({ template: '<RouterView />' }), {
    attachTo: document.body,
    global: { plugins: [router] },
  })
  await flushPromises()
  return wrapper
}

describe('OperatorLayout authentication boundary', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    eventSource.mockClear()
    loginWithHub.mockReset()
    vi.stubGlobal('EventSource', eventSource)
  })

  it('shows a sign-in gateway without mounting protected navigation or live data after 401', async () => {
    const wrapper = await mountOperator(401, { error: 'not logged in' })

    expect(wrapper.get('[data-operator-gateway]').text()).toContain('Sign in with Nimiq Hub')
    expect(wrapper.find('[data-operator-nav]').exists()).toBe(false)
    expect(wrapper.find('[data-section="attention"]').exists()).toBe(false)
    expect(eventSource).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('surfaces a clear denied state when the session is authenticated but not an operator', async () => {
    const wrapper = await mountOperator(403, { error: 'operator only' })

    expect(wrapper.get('[data-operator-gateway]').text()).toContain('This address is not an operator')
    expect(wrapper.get('[role="alert"]').text()).toContain('operator only')
    expect(wrapper.find('[data-operator-nav]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('mounts the console and live connection only after the operator session is accepted', async () => {
    const wrapper = await mountOperator(200, overviewResponse())

    expect(wrapper.get('[data-operator-nav]').element).toBeTruthy()
    expect(wrapper.get('[data-section="attention"]').element).toBeTruthy()
    expect(eventSource).toHaveBeenCalledTimes(1)

    wrapper.unmount()
  })
})
