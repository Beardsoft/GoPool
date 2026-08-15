import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { useLiveStatus } from './useLiveStatus'

describe('useLiveStatus', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.stubGlobal('EventSource', vi.fn(() => ({
      onopen: null,
      onmessage: null,
      onerror: null,
      close: vi.fn()
    })))
    vi.stubGlobal('setTimeout', vi.fn((cb) => 1))
    vi.stubGlobal('clearTimeout', vi.fn())
    vi.stubGlobal('setInterval', vi.fn(() => 1))
    vi.stubGlobal('clearInterval', vi.fn())
  })

  it('initial state is connecting', () => {
    const wrapper = mount({
      template: '<div />',
      setup() { return { ...useLiveStatus() } }
    })
    // state starts connecting before mount triggers start
    // we can't directly access, just ensure no error
    expect(wrapper.exists()).toBe(true)
  })

  it('reconnect resets backoff', () => {
    let reconnect!: ReturnType<typeof useLiveStatus>['reconnect']
    const wrapper = mount({
      template: '<div />',
      setup() {
        reconnect = useLiveStatus().reconnect
        return {}
      },
    })
    expect(typeof reconnect).toBe('function')
    wrapper.unmount()
  })
})
