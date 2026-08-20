import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import PoolOverview from './PoolOverview.vue'

describe('PoolOverview product composition', () => {
  it('combines the trust promise, live proof, staker lookup, reward explanation, and activity', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      current_epoch: 2,
      epoch_status: 'in_progress',
      num_stakers: 1,
      total_stake_luna: 10_100_000_000,
      total_rewards_luna: 177_489_526,
      pool_fee_percentage: 0.01,
      pool_name: 'GoPool Devnet',
      pool_description: 'Custom pool blurb for visiting stakers.',
      contact_url: '',
      disclosure: '',
      epoch_clock: {
        epoch: 145,
        head: 9256041,
        blocks_into_epoch: 3231,
        blocks_per_epoch: 43200,
        block_separation_ms: 1000,
        remaining_blocks: 39969,
        remaining_ms: 39_969_000,
      },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: PoolOverview },
        { path: '/stakers', component: { template: '<div />' } },
        { path: '/performance', component: { template: '<div />' } },
        { path: '/epochs', component: { template: '<div />' } },
      ],
    })
    await router.push('/')
    await router.isReady()

    const wrapper = mount(PoolOverview, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.get('[data-section="trust"]').text()).toContain('Custom pool blurb for visiting stakers.')
    expect(wrapper.get('[data-section="live-proof"]').text()).toContain('Current epoch')
    expect(wrapper.get('[data-section="live-proof"]').text()).toContain('11h 6m left')
    expect(wrapper.get('[data-section="staker-lookup"]').get('input').attributes('aria-label')).toBe('Nimiq address')
    expect(wrapper.get('[data-section="reward-model"]').text()).toMatch(/rewards|fee/i)
    expect(wrapper.get('[data-section="activity"]').text()).toMatch(/performance|activity/i)
  })

  it('sends unconfigured visitors to the setup wizard', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      code: 'setup_required',
      error: 'pool is not configured',
    }), { status: 503, headers: { 'Content-Type': 'application/json' } })))
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: PoolOverview },
        { path: '/setup', component: { template: '<div>setup</div>' } },
        { path: '/stakers', component: { template: '<div />' } },
        { path: '/performance', component: { template: '<div />' } },
        { path: '/epochs', component: { template: '<div />' } },
      ],
    })
    await router.push('/')
    await router.isReady()
    mount(PoolOverview, { global: { plugins: [router] } })
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/setup')
  })
})
