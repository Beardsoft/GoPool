import { vi, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { useTheme } from './useTheme'

it('uses system preference until manually overridden', () => {
  vi.stubGlobal('matchMedia', () => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
  let state!: ReturnType<typeof useTheme>
  const wrapper = mount({
    template: '<div />',
    setup() {
      state = useTheme()
      return {}
    },
  })
  expect(state.theme.value).toBe('dark')
  state.setPreference('light')
  expect(document.documentElement.dataset.theme).toBe('light')
  expect(localStorage.getItem('gopool-theme')).toBe('light')
  wrapper.unmount()
})
