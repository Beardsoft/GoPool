export function formatNim(luna: number): string {
  return (luna / 100_000).toLocaleString('en-US', { maximumFractionDigits: 5 })
}
export function formatPercent(fraction: number): string {
  return `${(fraction * 100).toFixed(2)}%`
}
export function shortAddress(address: string): string {
  const groups = address.trim().split(/\s+/)
  return groups.length >= 4 ? `${groups.slice(0, 2).join(' ')}…${groups.slice(-2).join(' ')}` : address
}
export function shortHash(hash: string): string {
  return hash.length >= 16 ? `${hash.slice(0, 8)}…${hash.slice(-6)}` : hash
}

export function formatRemaining(ms: number): string {
  if (ms <= 0) return 'epoch ending'
  const totalSec = Math.floor(ms / 1000)
  const hours = Math.floor(totalSec / 3600)
  const minutes = Math.floor((totalSec % 3600) / 60)
  const seconds = totalSec % 60
  if (hours > 0) {
    return minutes > 0 ? `${hours}h ${minutes}m left` : `${hours}h left`
  }
  if (minutes > 0) {
    return seconds > 0 ? `${minutes}m ${seconds}s left` : `${minutes}m left`
  }
  return `${seconds}s left`
}
