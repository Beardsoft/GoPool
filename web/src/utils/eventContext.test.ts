import { expect, it } from 'vitest'
import { compactEventFacts, parseEventContext } from './eventContext'

const payoutContext = JSON.stringify({
  address: 'NQ95 HH5Q QT81 0VE5 V9SA LCNY CV37 K6Q6 XMPM',
  amount: 21584135,
  fee: 0,
  kind: 'delegate',
  txHash: '807c040f8be37948fb9bcb344158f42d543676a1e4b44a7effc25aee3df0593b',
})

it('parses payout context into typed facts', () => {
  const facts = parseEventContext(payoutContext)
  expect(facts.map(fact => [fact.key, fact.kind, fact.label])).toEqual([
    ['address', 'address', 'Address'],
    ['amount', 'nim', 'Amount'],
    ['fee', 'nim', 'Fee'],
    ['kind', 'text', 'Kind'],
    ['txHash', 'tx', 'Tx'],
  ])
  expect(facts.find(fact => fact.key === 'amount')?.luna).toBe(21584135)
})

it('keeps compact overview facts to the useful payout details', () => {
  const compact = compactEventFacts(parseEventContext(payoutContext))
  expect(compact.map(fact => fact.key)).toEqual(['amount', 'address', 'kind', 'txHash'])
})

it('returns nothing for missing or invalid context', () => {
  expect(parseEventContext()).toEqual([])
  expect(parseEventContext('{')).toEqual([])
  expect(parseEventContext('[]')).toEqual([])
})
