import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { flushPromises } from '@vue/test-utils'
import Overview from './Overview.vue'
import { mockFetch, overviewFixture } from '../../test/helpers'

vi.stubGlobal('EventSource', vi.fn(() => ({ onopen: null, onmessage: null, onerror: null, close: vi.fn() })))

describe('Overview', () => {
  it('renders attention before telemetry', async () => {
    mockFetch('/api/operator/overview', overviewFixture({ status: 'attention' }))
    const wrapper = mount(Overview)
    await flushPromises()
    const attention = wrapper.get('[data-section="attention"]').element
    const telemetry = wrapper.get('[data-section="telemetry"]').element
    expect(attention.compareDocumentPosition(telemetry) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })
})
