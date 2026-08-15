import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { flushPromises } from '@vue/test-utils'
import Overview from './Overview.vue'
import { mockFetch, operatorTestGlobals, overviewFixture } from '../../test/helpers'

vi.stubGlobal('EventSource', vi.fn(() => ({ onopen: null, onmessage: null, onerror: null, close: vi.fn() })))

describe('Overview', () => {
  it('renders attention before telemetry', async () => {
    mockFetch('/api/operator/overview', overviewFixture({ status: 'attention' }))
    mockFetch('/api/pool', {
      current_epoch: 2, epoch_status: 'in_progress', num_stakers: 1,
      total_stake_luna: 10_100_000_000, total_rewards_luna: 177_489_526,
      pool_fee_percentage: 0.01,
    })
    const wrapper = mount(Overview, {
      global: {
        ...operatorTestGlobals,
        stubs: { RouterLink: { template: '<a href="#"><slot /></a>' } },
      },
    })
    await flushPromises()
    const attention = wrapper.get('[data-section="attention"]').element
    const telemetry = wrapper.get('[data-section="telemetry"]').element
    expect(attention.compareDocumentPosition(telemetry) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('renders the backend overview contract without blanking the authenticated console', async () => {
    mockFetch('/api/operator/overview', {
      status: 'attention',
      chain_lag: 2,
      wallet_runway_days: null,
      readiness: 'degraded',
      payout_summary: {},
      validator_summary: {
        address: 'NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E',
        state: 'active',
        last_processed_height: 816,
        last_tick_ms: 42,
      },
      attention: [{ id: 25, severity: 'error', category: 'rpc', summary: 'Failed to fetch block number' }],
      events: [{ id: 24, severity: 'info', category: 'checkpoint', summary: 'Reward collected for batch', created_at: '2026-08-15T20:57:52Z' }],
    })
    mockFetch('/api/pool', {
      current_epoch: 2,
      epoch_status: 'in_progress',
      num_stakers: 1,
      total_stake_luna: 10_100_000_000,
      total_rewards_luna: 177_489_526,
      pool_fee_percentage: 0.01,
    })

    const wrapper = mount(Overview, {
      global: {
        ...operatorTestGlobals,
        stubs: { RouterLink: { template: '<a href="#"><slot /></a>' } },
      },
    })
    await flushPromises()

    expect(wrapper.get('[role="status"]').text()).toContain('needs attention')
    expect(wrapper.get('[data-section="validator"]').text()).toContain('NQ20')
    expect(wrapper.get('[data-section="attention"]').text()).toContain('Failed to fetch block number')
  })

  it('keeps the daily overview scannable when many attention items are open', async () => {
    mockFetch('/api/operator/overview', overviewFixture({
      status: 'attention',
      attention: Array.from({ length: 8 }, (_, index) => ({
        id: index + 1,
        severity: 'error',
        category: 'readiness',
        summary: `Readiness issue ${index + 1}`,
      })),
    }))
    mockFetch('/api/pool', {
      current_epoch: 2, epoch_status: 'in_progress', num_stakers: 1,
      total_stake_luna: 10_100_000_000, total_rewards_luna: 177_489_526,
      pool_fee_percentage: 0.01,
    })
    const wrapper = mount(Overview, {
      global: {
        ...operatorTestGlobals,
        stubs: { RouterLink: { template: '<a href="#"><slot /></a>' } },
      },
    })
    await flushPromises()

    expect(wrapper.findAll('[data-section="attention"] li')).toHaveLength(5)
    expect(wrapper.get('[data-attention-overflow]').text()).toContain('3 more')
  })
})
