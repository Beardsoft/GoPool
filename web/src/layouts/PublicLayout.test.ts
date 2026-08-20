import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import PublicLayout from './PublicLayout.vue'
import { mockFetch } from '../test/helpers'
import { resetSessionCache } from '../composables/useSession'
import { resetExplorerForTests } from '../composables/useExplorer'

function routerForPublic() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: '/',
        component: PublicLayout,
        children: [
          { path: '', component: { template: '<div />' } },
          { path: 'performance', component: { template: '<div />' } },
          { path: 'stakers', component: { template: '<div />' } },
          { path: 'operator', component: { template: '<div />' } },
          { path: 'me', component: { template: '<div />' } },
        ],
      },
    ],
  })
}

describe('PublicLayout profile', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
    vi.stubGlobal('matchMedia', () => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))
    resetSessionCache()
    resetExplorerForTests()
    mockFetch('/api/session', { error: 'not logged in' }, 401)
  })

  async function mountPublic() {
    const router = routerForPublic()
    await router.push('/')
    await router.isReady()
    const wrapper = mount(defineComponent({ template: '<RouterView />' }), {
      global: { plugins: [router] },
    })
    await flushPromises()
    return wrapper
  }

  it('publishes the configured contact URL and disclosure', async () => {
    mockFetch('/api/pool', {
      pool_name: 'Aurora Stake',
      pool_description: 'We run a public Nimiq validator.',
      contact_url: 'https://aurora.example/contact',
      disclosure: 'Operated by Aurora Labs. Rewards are not guaranteed.',
      network: 'test-albatross',
    })
    const wrapper = await mountPublic()
    const contact = wrapper.get('[data-profile="contact"]')
    expect(contact.attributes('href')).toBe('https://aurora.example/contact')
    expect(contact.text()).toMatch(/contact/i)
    expect(wrapper.get('[data-profile="disclosure"]').text())
      .toContain('Operated by Aurora Labs. Rewards are not guaranteed.')
  })

  it('hides contact and disclosure when they are empty', async () => {
    mockFetch('/api/pool', {
      pool_name: 'GoPool',
      pool_description: '',
      contact_url: '',
      disclosure: '',
      network: 'test-albatross',
    })
    const wrapper = await mountPublic()
    expect(wrapper.find('[data-profile="contact"]').exists()).toBe(false)
    expect(wrapper.find('[data-profile="disclosure"]').exists()).toBe(false)
  })

  it('publishes configured telegram, discord, and x links', async () => {
    mockFetch('/api/pool', {
      pool_name: 'GoPool',
      contact_url: 'https://github.com/Beardsoft/GoPool',
      telegram_url: 'https://t.me/gopool',
      discord_url: 'https://discord.gg/gopool',
      x_url: 'https://x.com/gopool',
      disclosure: '',
      network: 'test-albatross',
    })
    const wrapper = await mountPublic()
    expect(wrapper.get('[data-profile="telegram"]').attributes('href')).toBe('https://t.me/gopool')
    expect(wrapper.get('[data-profile="discord"]').attributes('href')).toBe('https://discord.gg/gopool')
    expect(wrapper.get('[data-profile="x"]').attributes('href')).toBe('https://x.com/gopool')
  })

  it('hides social links when they are empty', async () => {
    mockFetch('/api/pool', {
      pool_name: 'GoPool',
      contact_url: 'https://github.com/Beardsoft/GoPool',
      telegram_url: '',
      discord_url: '',
      x_url: '',
      network: 'test-albatross',
    })
    const wrapper = await mountPublic()
    expect(wrapper.find('[data-profile="telegram"]').exists()).toBe(false)
    expect(wrapper.find('[data-profile="discord"]').exists()).toBe(false)
    expect(wrapper.find('[data-profile="x"]').exists()).toBe(false)
    expect(wrapper.find('[data-profile="contact"]').exists()).toBe(true)
  })
})
