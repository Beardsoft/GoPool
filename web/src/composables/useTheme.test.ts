import { vi, it, expect } from 'vitest'
import { useTheme } from './useTheme'

it('uses system preference until manually overridden', () => {
  vi.stubGlobal('matchMedia', () => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
  const state = useTheme()
  expect(state.theme.value).toBe('dark')
  state.setPreference('light')
  expect(document.documentElement.dataset.theme).toBe('light')
  expect(localStorage.getItem('gopool-theme')).toBe('light')
})
