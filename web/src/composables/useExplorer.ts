import { computed, ref } from 'vue'
import { apiGet } from '../api'
import type { PoolStatus } from '../types/api'

const network = ref('')
const poolName = ref('GoPool')
const contactUrl = ref('')
const telegramUrl = ref('')
const discordUrl = ref('')
const xUrl = ref('')
const disclosure = ref('')
let loaded = false
let pending: Promise<void> | null = null

export function loadNetwork(): Promise<void> {
  if (loaded) return Promise.resolve()
  if (!pending) {
    pending = apiGet<PoolStatus>('/api/pool')
      .then((pool) => {
        network.value = pool.network ?? ''
        poolName.value = pool.pool_name || 'GoPool'
        contactUrl.value = pool.contact_url ?? ''
        telegramUrl.value = pool.telegram_url ?? ''
        discordUrl.value = pool.discord_url ?? ''
        xUrl.value = pool.x_url ?? ''
        disclosure.value = pool.disclosure ?? ''
      })
      .catch(() => { network.value = '' })
      .finally(() => { loaded = true })
  }
  return pending
}

export function usePoolProfile() {
  return { poolName, contactUrl, telegramUrl, discordUrl, xUrl, disclosure }
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
  poolName.value = 'GoPool'
  contactUrl.value = ''
  telegramUrl.value = ''
  discordUrl.value = ''
  xUrl.value = ''
  disclosure.value = ''
  loaded = false
  pending = null
}
