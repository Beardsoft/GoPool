import { computed, ref } from 'vue'
import { apiGet, apiPost } from '../api'
import { loginWithHub } from '../hub'

interface SessionInfo {
  signedIn: boolean
  address: string
  operator: boolean
}

const signedOut: SessionInfo = { signedIn: false, address: '', operator: false }
let cache: Promise<SessionInfo> | null = null

function fetchSession(): Promise<SessionInfo> {
  if (!cache) {
    cache = apiGet<{ address: string; operator: boolean }>('/api/session')
      .then((s) => ({ signedIn: true, address: s.address, operator: s.operator }))
      .catch(() => ({ ...signedOut }))
  }
  return cache
}

export function useSession() {
  const state = ref<SessionInfo>({ ...signedOut })
  fetchSession().then((s) => { state.value = s })

  return {
    signedIn: computed(() => state.value.signedIn),
    address: computed(() => state.value.address),
    operator: computed(() => state.value.operator),
    async login() {
      await loginWithHub()
      cache = null
      state.value = await fetchSession()
    },
    async logout() {
      await apiPost('/api/auth/logout')
      cache = null
      state.value = { ...signedOut }
    },
  }
}

export function resetSessionCache() {
  cache = null
}
