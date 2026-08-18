import { expect, it } from 'vitest'
import { formatNim, formatPercent, shortAddress, shortHash } from './format'

it('formats Nimiq values from luna', () => {
  expect(formatNim(1_842_193_612_345)).toBe('18,421,936.12345')
  expect(formatNim(100_000)).toBe('1')
  expect(formatPercent(0.025)).toBe('2.50%')
  expect(shortAddress('NQ12 8D4K AAAA BBBB CCCC DDDD EEEE FFFF GGGG')).toBe('NQ12 8D4K…FFFF GGGG')
  expect(shortHash('807c040f8be37948fb9bcb344158f42d543676a1e4b44a7effc25aee3df0593b')).toBe('807c040f…f0593b')
})
