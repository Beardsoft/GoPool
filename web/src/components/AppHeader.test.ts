import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AppHeader from './AppHeader.vue'

describe('AppHeader theme control', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
    vi.stubGlobal('matchMedia', () => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))
  })

  it('switches the complete application theme and persists the choice', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: { template: '<div />' } },
        { path: '/performance', component: { template: '<div />' } },
        { path: '/stakers', component: { template: '<div />' } },
        { path: '/operator', component: { template: '<div />' } },
      ],
    })
    await router.push('/')
    await router.isReady()
    const wrapper = mount(AppHeader, { global: { plugins: [router] } })

    const toggle = wrapper.get('[aria-label="Switch to dark theme"]')
    await toggle.trigger('click')

    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(localStorage.getItem('gopool-theme')).toBe('dark')
    expect(wrapper.get('[aria-label="Switch to light theme"]').element).toBeTruthy()
  })
})
