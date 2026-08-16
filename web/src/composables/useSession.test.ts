import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useSession, resetSessionCache } from './useSession'
import { loginWithHub } from '../hub'
import { mockFetch } from '../test/helpers'

const ADDR = 'NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE'

vi.mock('../hub', () => ({ loginWithHub: vi.fn() }))

const settle = () => new Promise((resolve) => setTimeout(resolve, 0))

describe('useSession', () => {
  beforeEach(() => {
    resetSessionCache()
    vi.mocked(loginWithHub).mockReset()
  })

  it('reports signed out when the session endpoint rejects', async () => {
    mockFetch('/api/session', { error: 'not logged in' }, 401)
    const { signedIn, address } = useSession()
    await settle()
    expect(signedIn.value).toBe(false)
    expect(address.value).toBe('')
  })

  it('exposes the session address and operator flag', async () => {
    mockFetch('/api/session', { address: ADDR, operator: true })
    const { signedIn, address, operator } = useSession()
    await settle()
    expect(signedIn.value).toBe(true)
    expect(address.value).toBe(ADDR)
    expect(operator.value).toBe(true)
  })

  it('login signs in via Hub and re-fetches the session', async () => {
    vi.mocked(loginWithHub).mockResolvedValue({ address: ADDR })
    mockFetch('/api/session', { address: ADDR, operator: false })
    const { signedIn, login } = useSession()
    await login()
    expect(loginWithHub).toHaveBeenCalledTimes(1)
    expect(signedIn.value).toBe(true)
  })

  it('logout clears the session state', async () => {
    mockFetch('/api/session', { address: ADDR, operator: false })
    mockFetch('/api/auth/logout', { ok: true })
    const { signedIn, logout } = useSession()
    await settle()
    expect(signedIn.value).toBe(true)
    await logout()
    expect(signedIn.value).toBe(false)
  })
})
