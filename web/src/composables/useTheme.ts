import { ref, computed, onMounted, watch } from 'vue'

type ThemePreference = 'system' | 'light' | 'dark'

const preferenceKey = 'gopool-theme'

function getSystemTheme(): 'light' | 'dark' {
  const mm = (globalThis as any).matchMedia || (typeof window !== 'undefined' && window.matchMedia)
  if (!mm) return 'light'
  return mm('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function useTheme() {
  const preference = ref<ThemePreference>('system')

  const theme = computed<'light' | 'dark'>(() => {
    if (preference.value === 'system') return getSystemTheme()
    return preference.value
  })

  function applyTheme() {
    if (typeof document !== 'undefined') {
      document.documentElement.dataset.theme = theme.value
    }
  }

  function setPreference(pref: ThemePreference) {
    preference.value = pref
    try { localStorage.setItem(preferenceKey, pref) } catch {}
    applyTheme()
  }

  function toggleTheme() {
    const next = theme.value === 'dark' ? 'light' : 'dark'
    setPreference(next)
  }

  onMounted(() => {
    try {
      const saved = localStorage.getItem(preferenceKey) as ThemePreference | null
      if (saved) preference.value = saved
    } catch {}
    applyTheme()
    if (typeof window !== 'undefined' && window.matchMedia) {
      const media = window.matchMedia('(prefers-color-scheme: dark)')
      media.addEventListener('change', () => {
        if (preference.value === 'system') applyTheme()
      })
    }
  })

  watch(theme, applyTheme)

  return {
    theme,
    preference,
    setPreference,
    toggleTheme,
  }
}
