import type { OperatorEvent } from '../types/api'

export type EventFactKind = 'nim' | 'address' | 'tx' | 'text'

export interface EventFact {
  key: string
  label: string
  raw: string
  kind: EventFactKind
  luna?: number
}

const LUNA_KEYS = new Set([
  'amount', 'fee', 'poolfee', 'afterfee', 'rewardluna', 'balance', 'livestake',
  'pendingpayoutluna', 'stuckpayoutluna', 'minpayoutluna',
])

const ADDRESS_KEYS = new Set(['address', 'derived'])
const TX_KEYS = new Set(['txhash'])
const ADDRESS_RE = /^NQ[0-9A-Z]{2}( [0-9A-Z]{4})+$/
const TX_RE = /^[0-9a-fA-F]{64}$/

const LABELS: Record<string, string> = {
  amount: 'Amount',
  fee: 'Fee',
  address: 'Address',
  derived: 'Wallet',
  txhash: 'Tx',
  kind: 'Kind',
  epoch: 'Epoch',
  batch: 'Batch',
  height: 'Height',
  numstakers: 'Stakers',
  rewardluna: 'Reward',
  afterfee: 'After fee',
  poolfee: 'Pool fee',
  balance: 'Balance',
  livestake: 'Stake',
  error: 'Error',
  action: 'Action',
  status: 'Status',
}

const COMPACT_ORDER = [
  'amount', 'rewardluna', 'afterfee', 'balance', 'livestake',
  'address', 'kind', 'action', 'txhash', 'epoch', 'batch', 'numstakers', 'error',
]

export function parseEventContext(contextJson?: string | null): EventFact[] {
  if (!contextJson) return []
  try {
    const parsed = JSON.parse(contextJson)
    if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) return []
    return Object.entries(parsed).flatMap(([key, value]) => {
      const fact = toFact(key, value)
      return fact ? [fact] : []
    })
  } catch {
    return []
  }
}

export function eventFacts(event: Pick<OperatorEvent, 'context_json'>, compact = false): EventFact[] {
  const facts = parseEventContext(event.context_json)
  return compact ? compactEventFacts(facts) : facts
}

export function compactEventFacts(facts: EventFact[], limit = 4): EventFact[] {
  const skip = new Set<string>()
  for (const fact of facts) {
    if (fact.key.toLowerCase() === 'fee' && fact.luna === 0) skip.add(fact.key)
  }
  const byKey = new Map(facts.map(fact => [fact.key.toLowerCase(), fact]))
  const out: EventFact[] = []
  for (const key of COMPACT_ORDER) {
    const fact = byKey.get(key)
    if (fact && !skip.has(fact.key)) out.push(fact)
    if (out.length >= limit) return out
  }
  for (const fact of facts) {
    if (out.includes(fact) || skip.has(fact.key)) continue
    out.push(fact)
    if (out.length >= limit) break
  }
  return out
}

function toFact(key: string, value: unknown): EventFact | null {
  if (value == null || value === '') return null
  const kind = classify(key, value)
  const luna = kind === 'nim' ? asNumber(value) : undefined
  if (kind === 'nim' && luna == null) return null
  const raw = kind === 'nim' ? String(luna) : stringify(value)
  if (!raw) return null
  return { key, label: labelFor(key), raw, kind, luna: luna ?? undefined }
}

function classify(key: string, value: unknown): EventFactKind {
  const normalized = key.toLowerCase()
  const text = stringify(value)
  if (normalized.endsWith('luna') || LUNA_KEYS.has(normalized)) return 'nim'
  if (ADDRESS_KEYS.has(normalized) || ADDRESS_RE.test(text)) return 'address'
  if (TX_KEYS.has(normalized) || TX_RE.test(text)) return 'tx'
  return 'text'
}

function labelFor(key: string): string {
  const known = LABELS[key.toLowerCase()]
  if (known) return known
  return key.replace(/([a-z])([A-Z])/g, '$1 $2').replace(/^./, char => char.toUpperCase())
}

function asNumber(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string' && /^-?\d+$/.test(value)) return Number(value)
  return null
}

function stringify(value: unknown): string {
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  try {
    return JSON.stringify(value)
  } catch {
    return ''
  }
}
