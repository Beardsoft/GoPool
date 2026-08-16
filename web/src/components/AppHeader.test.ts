import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AppHeader from './AppHeader.vue'
import { mockFetch } from '../test/helpers'
import { resetSessionCache } from '../composables/useSession'

const ADDR = 'NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE'

function routerForHeader() {
  const placeholder = { template: '<div />' }
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: placeholder },
      { path: '/performance', component: placeholder },
      { path: '/stakers', component: placeholder },
      { path: '/operator', component: placeholder },
      { path: '/me', component: placeholder },
    ],
  })
}

describe('AppHeader', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
    vi.stubGlobal('matchMedia', () => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))
    resetSessionCache()
    mockFetch('/api/session', { error: 'not logged in' }, 401)
  })

  async function mountHeader(session?: { address: string; operator: boolean }) {
    if (session) mockFetch('/api/session', session)
    const router = routerForHeader()
    await router.push('/')
    await router.isReady()
    const wrapper = mount(AppHeader, { global: { plugins: [router] } })
    await new Promise((resolve) => setTimeout(resolve, 0))
    return wrapper
  }

  it('shows a sign-in button when signed out', async () => {
    const wrapper = await mountHeader()
    expect(wrapper.get('.login-btn').text()).toBe('Sign in with Nimiq')
    expect(wrapper.find('.session-chip').exists()).toBe(false)
    expect(wrapper.find('.operator-link').exists()).toBe(true)
  })

  it('shows the session chip and keeps the operator link for operators', async () => {
    const wrapper = await mountHeader({ address: ADDR, operator: true })
    expect(wrapper.get('.session-address').attributes('title')).toBe(ADDR)
    expect(wrapper.get('.session-address').text()).toContain('…')
    expect(wrapper.find('img.identicon').exists()).toBe(true)
    expect(wrapper.find('.operator-link').exists()).toBe(true)
    expect(wrapper.find('.login-btn').exists()).toBe(false)
  })

  it('hides the operator link for signed-in non-operators', async () => {
    const wrapper = await mountHeader({ address: ADDR, operator: false })
    expect(wrapper.find('.operator-link').exists()).toBe(false)
  })

  it('switches the complete application theme and persists the choice', async () => {
    const wrapper = await mountHeader()
    const toggle = wrapper.get('[aria-label="Switch to dark theme"]')
    await toggle.trigger('click')
    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(localStorage.getItem('gopool-theme')).toBe('dark')
    expect(wrapper.get('[aria-label="Switch to light theme"]').element).toBeTruthy()
  })
})
