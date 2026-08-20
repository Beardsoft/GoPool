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

  it('surfaces payout context on recent activity rows', async () => {
    mockFetch('/api/operator/overview', overviewFixture({
      events: [{
        id: 24,
        severity: 'info',
        category: 'payout',
        summary: 'Payout submitted',
        created_at: '2026-08-18T06:31:45Z',
        context_json: JSON.stringify({
          address: 'NQ95 HH5Q QT81 0VE5 V9SA LCNY CV37 K6Q6 XMPM',
          amount: 21584135,
          fee: 0,
          kind: 'delegate',
          txHash: '807c040f8be37948fb9bcb344158f42d543676a1e4b44a7effc25aee3df0593b',
        }),
      }],
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
    const row = wrapper.get('[data-section="activity"] li')
    expect(row.text()).toContain('Payout submitted')
    expect(row.text()).toContain('215.84135 NIM')
    expect(row.text()).toContain('NQ95 HH5Q…K6Q6 XMPM')
    expect(row.text()).toContain('delegate')
    expect(row.text()).not.toContain('21584135')
    expect(row.find('[aria-label="Copy wallet address"]').exists()).toBe(true)
    expect(row.find('[aria-label="Copy transaction hash"]').exists()).toBe(true)
    expect(row.get('.event-facts').attributes('data-layout')).toBe('compact')
    expect(row.get('.event-facts').attributes('data-count')).toBe('4')
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

  it('shows elected status and slot count on the metrics strip', async () => {
    mockFetch('/api/operator/overview', overviewFixture({
      epoch_participation: { epoch: 2, elected: true, slot_count: 12, slots_total: 512 },
    }))
    mockFetch('/api/pool', {
      current_epoch: 2, epoch_status: 'in_progress', num_stakers: 1,
      total_stake_luna: 0, total_rewards_luna: 0, pool_fee_percentage: 0.01,
      epoch_clock: { epoch: 2, head: 1241, blocks_into_epoch: 1, blocks_per_epoch: 240, block_separation_ms: 1000, remaining_blocks: 239, remaining_ms: 239_000 },
    })
    const wrapper = mount(Overview, {
      global: {
        ...operatorTestGlobals,
        stubs: { RouterLink: { template: '<a href="#"><slot /></a>' } },
      },
    })
    await flushPromises()
    const metrics = wrapper.get('.metrics').text()
    expect(metrics).toContain('Elected')
    expect(metrics).toContain('Epoch 2')
    expect(metrics).toContain('12')
    expect(metrics).toContain('of 512')
    expect(metrics).toContain('3m 59s left')
  })

  it('shows placeholders before the first epoch snapshot', async () => {
    mockFetch('/api/operator/overview', overviewFixture())
    mockFetch('/api/pool', {
      current_epoch: 2, epoch_status: 'in_progress', num_stakers: 1,
      total_stake_luna: 0, total_rewards_luna: 0, pool_fee_percentage: 0.01,
    })
    const wrapper = mount(Overview, {
      global: {
        ...operatorTestGlobals,
        stubs: { RouterLink: { template: '<a href="#"><slot /></a>' } },
      },
    })
    await flushPromises()
    expect(wrapper.get('.metrics').text()).toMatch(/—/)
    expect(wrapper.get('.metrics').text()).not.toContain('Not elected')
  })

  it('lists missing stake as attention instead of all-clear', async () => {
    mockFetch('/api/operator/overview', overviewFixture({
      status: 'attention',
      readiness: 'error',
      validator_summary: {
        address: 'NQ51 6A35 D8G7 19B9 U8LX TJ7S XLLN H4VT Q910',
        state: 'unready',
        last_processed_height: 0,
        last_tick_ms: 1,
      },
      attention: [{
        id: 0,
        severity: 'warning',
        category: 'readiness',
        summary: 'Waiting for 100000 NIM to register the validator (have 0 NIM)',
      }],
    }))
    mockFetch('/api/pool', {
      current_epoch: 0, epoch_status: 'in_progress', num_stakers: 0,
      total_stake_luna: 0, total_rewards_luna: 0, pool_fee_percentage: 0.01,
    })
    const wrapper = mount(Overview, {
      global: {
        ...operatorTestGlobals,
        stubs: { RouterLink: { template: '<a href="#"><slot /></a>' } },
      },
    })
    await flushPromises()
    const attention = wrapper.get('[data-section="attention"]')
    expect(attention.text()).toContain('Waiting for 100000 NIM to register the validator (have 0 NIM)')
    expect(attention.text()).not.toContain('Nothing requires action')
    expect(wrapper.get('[role="status"]').text()).toContain('needs attention')
  })
})
