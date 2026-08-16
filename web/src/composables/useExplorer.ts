import { computed, ref } from 'vue'
import { apiGet } from '../api'

const network = ref('')
let loaded = false
let pending: Promise<void> | null = null

export function loadNetwork(): Promise<void> {
  if (loaded) return Promise.resolve()
  if (!pending) {
    pending = apiGet<{ network?: string }>('/api/pool')
      .then((pool) => { network.value = pool.network ?? '' })
      .catch(() => { network.value = '' })
      .finally(() => { loaded = true })
  }
  return pending
}

const explorerBase = computed(() => {
  const n = network.value
  if (n.includes('main')) return 'https://nimiqscan.com'
  if (n.includes('test')) return 'https://testnet.nimiqscan.com'
  return null
})

export type ExplorerKind = 'account' | 'transaction'

export function useExplorer() {
  const txUrl = (hash?: string | null): string | null =>
    hash && explorerBase.value ? `${explorerBase.value}/transaction/${hash}` : null

  const accountUrl = (address?: string | null): string | null =>
    address && explorerBase.value ? `${explorerBase.value}/account/${encodeURIComponent(address)}` : null

  const explorerUrlFor = (value: string): { kind: ExplorerKind; url: string } | null => {
    if (!explorerBase.value || !value) return null
    if (/^[0-9a-fA-F]{64}$/.test(value)) return { kind: 'transaction', url: txUrl(value)! }
    if (/^NQ[0-9A-Z]{2}( [0-9A-Z]{4})*$/.test(value)) return { kind: 'account', url: accountUrl(value)! }
    return null
  }

  return { network, explorerBase, txUrl, accountUrl, explorerUrlFor }
}

export function resetExplorerForTests() {
  network.value = ''
  loaded = false
  pending = null
}
