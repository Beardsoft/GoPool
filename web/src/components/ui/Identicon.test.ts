import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import Identicon from './Identicon.vue'

const ADDR = 'NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE'

describe('Identicon', () => {
  it('renders a data-url identicon for a valid address', async () => {
    const wrapper = mount(Identicon, { props: { address: ADDR } })
    await flushPromises()
    const img = wrapper.get('img')
    expect((img.element as HTMLImageElement).src.startsWith('data:image/svg+xml;base64,')).toBe(true)
  })

  it('renders nothing for an invalid address', async () => {
    const wrapper = mount(Identicon, { props: { address: 'not-an-address' } })
    await flushPromises()
    expect(wrapper.find('img').exists()).toBe(false)
  })
})
