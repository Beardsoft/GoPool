import { ref, onMounted, onUnmounted } from 'vue'
import { apiGet } from '../api'

export type LiveState = 'connecting' | 'live' | 'paused'

export function useLiveStatus() {
  const state = ref<LiveState>('connecting')
  const lastEventAt = ref<number | null>(null)
  let es: EventSource | null = null
  let backoff = 1000
  let reconnectTimer: number | null = null

  function start() {
    if (es) es.close()
    es = new EventSource('/api/operator/events')
    es.onopen = () => {
      state.value = 'live'
      backoff = 1000
    }
    es.onmessage = () => {
      lastEventAt.value = Date.now()
    }
    es.onerror = () => {
      es?.close()
      es = null
      state.value = 'paused'
      // bounded backoff
      if (!reconnectTimer) {
        reconnectTimer = window.setTimeout(() => {
          reconnectTimer = null
          backoff = Math.min(backoff * 2, 30000)
          start()
        }, backoff)
      }
    }
  }

  function reconnect() {
    backoff = 1000
    start()
    // trigger authoritative refetch
    // Note: caller may refetch via apiGet
  }

  onMounted(() => {
    start()
  })

  onUnmounted(() => {
    es?.close()
    if (reconnectTimer) clearTimeout(reconnectTimer)
  })

  return { state, lastEventAt, reconnect }
}
