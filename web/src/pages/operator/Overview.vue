<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { apiGet } from '../../api'
import { useLiveStatus } from '../../composables/useLiveStatus'
import type { OperatorOverview, PoolStatus, TelemetryPoint } from '../../types/api'
import ExplorerLink from '../../components/ui/ExplorerLink.vue'
import NimAmount from '../../components/ui/NimAmount.vue'
import AddressIdentity from '../../components/ui/AddressIdentity.vue'
import TelemetryChart from '../../components/TelemetryChart.vue'
import EventFacts from '../../components/EventFacts.vue'
import { eventFacts } from '../../utils/eventContext'
import { formatRemaining } from '../../utils/format'

const overview = ref<OperatorOverview | null>(null)
const pool = ref<PoolStatus | null>(null)
const telemetryPoints = ref<TelemetryPoint[]>([])
const loading = ref(true)
const error = ref('')

const { state, lastEventAt, reconnect } = useLiveStatus()
const passingChecks = computed(() => {
  if (!overview.value) return 0
  return [overview.value.readiness === 'ok', overview.value.chain_lag <= 2, overview.value.validator_summary.state === 'active', overview.value.attention.length === 0].filter(Boolean).length
})
const healthLabel = computed(() => overview.value?.status === 'healthy' ? 'Pool is healthy' : 'Pool needs attention')
const visibleAttention = computed(() => overview.value?.attention.slice(0, 5) ?? [])
const hiddenAttentionCount = computed(() => Math.max(0, (overview.value?.attention.length ?? 0) - visibleAttention.value.length))
const visibleEvents = computed(() => overview.value?.events.slice(0, 8) ?? [])
const epochElectedLabel = computed(() => {
  const elected = overview.value?.epoch_participation?.elected
  if (elected == null) return '—'
  return elected ? 'Elected' : 'Not elected'
})
const epochNumberLabel = computed(() => overview.value?.epoch_participation?.epoch ?? pool.value?.epoch_clock?.epoch ?? '—')
const slotCountLabel = computed(() => overview.value?.epoch_participation?.slot_count ?? '—')
const slotsTotalLabel = computed(() => overview.value?.epoch_participation?.slots_total ?? '—')
const epochRemainingLabel = computed(() => {
  const ms = pool.value?.epoch_clock?.remaining_ms
  return ms == null ? '' : formatRemaining(ms)
})

async function load() {
  try {
    const [operatorOverview, poolStatus] = await Promise.all([
      apiGet<OperatorOverview>('/api/operator/overview'),
      apiGet<PoolStatus>('/api/pool'),
    ])
    overview.value = operatorOverview
    pool.value = poolStatus
    error.value = ''
    const to = new Date()
    const from = new Date(to.getTime() - 24 * 60 * 60 * 1000)
    apiGet<TelemetryPoint[]>(`/api/operator/telemetry?metric=chain_lag&from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}&bucket=15m`)
      .then(points => { telemetryPoints.value = points })
      .catch(() => { telemetryPoints.value = [] })
  } catch (cause) {
    error.value = (cause as Error).message || 'The latest pool state could not be loaded.'
  } finally {
    loading.value = false
  }
}

function reconnectAndRefresh() {
  reconnect()
  load()
}

onMounted(load)
</script>

<template>
  <div class="operator-overview">
    <div v-if="loading" class="overview-loading" aria-live="polite">Loading operator state…</div>
    <p v-else-if="error" class="error" role="alert">{{ error }}</p>

    <template v-if="overview">
      <section class="status-hero" :data-health="overview.status">
        <div class="status-copy">
          <div class="status-icon" aria-hidden="true">
            <svg viewBox="0 0 24 24"><path d="m6 12 4 4 8-9"/></svg>
          </div>
          <div>
            <p class="status-kicker">Operator overview</p>
            <h1 role="status">{{ healthLabel }}</h1>
            <p>{{ passingChecks }} of 4 core readiness checks are passing.</p>
          </div>
        </div>
        <div class="live-control">
          <span :data-live="state"><i></i>{{ state === 'live' ? 'Live' : state === 'paused' ? 'Connection paused' : 'Connecting' }}</span>
          <button v-if="state === 'paused'" type="button" @click="reconnectAndRefresh">Reconnect</button>
          <small v-if="lastEventAt">Updated {{ new Date(lastEventAt).toLocaleTimeString() }}</small>
        </div>
      </section>

      <section class="metrics" aria-label="Operational metrics">
        <article>
          <p>This epoch</p>
          <strong>{{ epochElectedLabel }}</strong>
          <small>Epoch {{ epochNumberLabel }}<template v-if="epochRemainingLabel"> · {{ epochRemainingLabel }}</template></small>
        </article>
        <article>
          <p>Slots</p>
          <strong>{{ slotCountLabel }}</strong>
          <small>of {{ slotsTotalLabel }}</small>
        </article>
        <article>
          <p>Chain lag</p>
          <strong>{{ overview.chain_lag }}</strong>
          <small>blocks behind head</small>
        </article>
        <article>
          <p>Delegated stake</p>
          <strong><NimAmount :luna="pool?.total_stake_luna ?? 0" /></strong>
          <small>{{ pool?.num_stakers ?? 0 }} active staker{{ pool?.num_stakers === 1 ? '' : 's' }}</small>
        </article>
        <article>
          <p>Rewards processed</p>
          <strong><NimAmount :luna="pool?.total_rewards_luna ?? 0" /></strong>
          <small>cumulative pool record</small>
        </article>
        <article>
          <p>Wallet runway</p>
          <strong>{{ overview.wallet_runway_days ?? '—' }}<span v-if="overview.wallet_runway_days"> days</span></strong>
          <small>estimated fee capacity</small>
        </article>
      </section>

      <section data-section="attention" class="attention-section" :data-empty="overview.attention.length === 0">
        <div class="section-heading">
          <div>
            <p class="section-kicker">Needs attention</p>
            <h2>{{ overview.attention.length ? `${overview.attention.length} item${overview.attention.length === 1 ? '' : 's'} to review` : 'Nothing requires action' }}</h2>
          </div>
          <span class="attention-count">{{ overview.attention.length }}</span>
        </div>
        <ul v-if="overview.attention.length">
          <li v-for="event in visibleAttention" :key="event.id">
            <span class="event-severity" :data-severity="event.severity"></span>
            <div><strong>{{ event.summary }}</strong><small>{{ event.category }}<template v-if="event.created_at"> · {{ new Date(event.created_at).toLocaleString() }}</template></small></div>
          </li>
        </ul>
        <RouterLink v-if="hiddenAttentionCount" to="/operator/activity" class="attention-overflow" data-attention-overflow>
          {{ hiddenAttentionCount }} more item{{ hiddenAttentionCount === 1 ? '' : 's' }} in activity →
        </RouterLink>
        <div v-else-if="!overview.attention.length" class="all-clear">
          <span aria-hidden="true">✓</span>
          <p>Queues are moving, the validator is reporting, and no delivery failures are open.</p>
        </div>
      </section>

      <div class="detail-grid">
        <section data-section="validator" class="detail-section validator-section">
          <div class="section-heading compact">
            <div><p class="section-kicker">Validator</p><h2>Identity & state</h2></div>
            <span class="state-pill">{{ overview.validator_summary.state || 'unknown' }}</span>
          </div>
          <AddressIdentity :address="overview.validator_summary.address" copyable />
          <ExplorerLink kind="account" :value="overview.validator_summary.address" label="View on explorer" />
          <dl>
            <div><dt>Processed height</dt><dd>{{ overview.validator_summary.last_processed_height.toLocaleString() }}</dd></div>
            <div><dt>Last tick</dt><dd>{{ overview.validator_summary.last_tick_ms.toLocaleString() }} ms</dd></div>
            <div><dt>Current stake</dt><dd><NimAmount :luna="pool?.total_stake_luna ?? 0" /></dd></div>
            <div><dt>Delegators</dt><dd>{{ pool?.num_stakers ?? 0 }}</dd></div>
          </dl>
        </section>

        <section data-section="telemetry" class="detail-section telemetry-section">
          <div class="section-heading compact">
            <div><p class="section-kicker">Telemetry</p><h2>Chain lag · 24 hours</h2></div>
          </div>
          <TelemetryChart v-if="telemetryPoints.length" :points="telemetryPoints" metric="Chain lag" />
          <div v-else class="empty-telemetry"><span></span><p>No trend samples yet. Live snapshots appear after the daemon records its next intervals.</p></div>
        </section>
      </div>

      <section data-section="activity" class="activity-section">
        <div class="section-heading compact">
          <div><p class="section-kicker">Recent activity</p><h2>What the pool has been doing</h2></div>
          <RouterLink to="/operator/activity">View full activity →</RouterLink>
        </div>
        <ul v-if="visibleEvents.length">
          <li v-for="event in visibleEvents" :key="event.id">
            <span class="activity-dot" :data-severity="event.severity"></span>
            <div class="event-copy"><strong>{{ event.summary }}</strong><small>{{ event.source || event.category }}<template v-if="event.created_at"> · {{ new Date(event.created_at).toLocaleString() }}</template></small></div>
            <EventFacts :facts="eventFacts(event, true)" />
          </li>
        </ul>
        <p v-else class="empty-copy">No structured activity has been recorded yet.</p>
      </section>
    </template>
  </div>
</template>

<style scoped>
.operator-overview { display: grid; gap: 24px; }
.overview-loading { min-height: 240px; display: grid; place-items: center; color: var(--app-muted); }
.status-hero { display: flex; align-items: center; justify-content: space-between; gap: 32px; padding: 32px 36px; border-radius: 18px; color: white; background: var(--nimiq-green-bg); box-shadow: 0 20px 44px rgba(33,188,165,.2); }
.status-hero[data-health='attention'] { background: var(--nimiq-gold-bg); }.status-hero[data-health='degraded'] { background: var(--nimiq-red-bg); }
.status-copy { display: flex; align-items: center; gap: 22px; }.status-icon { width: 56px; height: 60px; display: grid; place-items: center; flex: 0 0 auto; background: rgba(255,255,255,.15); clip-path: polygon(25% 6%,75% 6%,100% 50%,75% 94%,25% 94%,0 50%); }.status-icon svg { width: 29px; fill: none; stroke: currentColor; stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; }
.status-kicker { margin: 0 0 5px; color: rgba(255,255,255,.65); font-size: .76rem; font-weight: 800; }.status-copy h1 { margin: 0 0 6px; font-size: clamp(1.7rem, 3.6vw, 2.5rem); letter-spacing: -.035em; }.status-copy p:last-child { margin: 0; color: rgba(255,255,255,.76); }
.live-control { display: grid; justify-items: end; gap: 8px; }.live-control > span { display: inline-flex; align-items: center; gap: 8px; padding: 8px 12px; border-radius: 999px; background: rgba(255,255,255,.14); font-size: .78rem; font-weight: 800; }.live-control i { width: 7px; height: 7px; border-radius: 50%; background: white; box-shadow: 0 0 0 4px rgba(255,255,255,.15); }.live-control [data-live='paused'] i { background: var(--nimiq-gold); }.live-control button { border: 0; color: white; background: none; text-decoration: underline; cursor: pointer; }.live-control small { color: rgba(255,255,255,.56); }
.metrics { display: grid; grid-template-columns: repeat(3, 1fr); border-radius: 16px; background: var(--surface-1); box-shadow: var(--shadow-elevated); }.metrics article { min-width: 0; padding: 26px; border-right: 1px solid var(--app-border); border-bottom: 1px solid var(--app-border); }.metrics article:nth-child(3n) { border-right: 0; }.metrics article:nth-last-child(-n+3) { border-bottom: 0; }.metrics p { margin: 0 0 9px; color: var(--app-faint); font-size: .76rem; font-weight: 800; }.metrics strong { display: block; overflow: hidden; font-size: 1.35rem; letter-spacing: -.025em; text-overflow: ellipsis; }.metrics strong span { font-size: .9rem; color: var(--app-muted); }.metrics small { display: block; margin-top: 7px; color: var(--app-faint); }
.attention-section, .detail-section, .activity-section { padding: 30px; border-radius: 16px; background: var(--surface-1); box-shadow: var(--shadow-elevated); }.attention-section[data-empty='true'] { background: var(--success-soft); box-shadow: none; }
.section-heading { display: flex; align-items: center; justify-content: space-between; gap: 24px; margin-bottom: 24px; }.section-heading.compact { margin-bottom: 22px; }.section-kicker { margin: 0 0 6px; color: var(--app-faint); font-size: .75rem; font-weight: 800; }.section-heading h2 { margin: 0; font-size: 1.35rem; letter-spacing: -.025em; }.attention-count { width: 40px; height: 40px; display: grid; place-items: center; border-radius: 50%; background: var(--surface-2); font-weight: 800; }
.attention-section ul, .activity-section ul { display: grid; gap: 0; margin: 0; padding: 0; list-style: none; }.attention-section li, .activity-section li { display: flex; align-items: flex-start; gap: 14px; padding: 15px 0; border-top: 1px solid var(--app-border); }.activity-section li { display: grid; grid-template-columns: 9px 17.5rem minmax(0, 1fr); align-items: start; column-gap: 20px; row-gap: 10px; }.event-severity, .activity-dot { width: 9px; height: 9px; margin-top: 6px; flex: 0 0 auto; border-radius: 50%; background: var(--nimiq-gold); }.event-severity[data-severity='error'], .activity-dot[data-severity='error'] { background: var(--nimiq-red); }.event-severity[data-severity='info'], .activity-dot[data-severity='info'] { background: var(--nimiq-light-blue); }.attention-section li div, .activity-section .event-copy { display: grid; min-width: 0; }.attention-section li small, .activity-section .event-copy small { margin-top: 4px; color: var(--app-faint); }
.attention-overflow { display: inline-flex; margin-top: 18px; color: var(--nimiq-light-blue); font-size: .86rem; font-weight: 800; text-decoration: none; }
.all-clear { display: flex; align-items: center; gap: 14px; color: var(--success-text); }.all-clear span { width: 34px; height: 34px; display: grid; place-items: center; flex: 0 0 auto; border-radius: 50%; color: white; background: var(--nimiq-green); font-weight: 900; }.all-clear p { margin: 0; }
.detail-grid { display: grid; grid-template-columns: .9fr 1.1fr; gap: 24px; }.state-pill { padding: 7px 11px; border-radius: 999px; color: var(--success-text); background: var(--success-soft); font-size: .76rem; font-weight: 800; text-transform: capitalize; }.validator-section :deep(.address-identity) { max-width: 100%; padding: 12px 14px; border-radius: 10px; background: var(--surface-2); }.validator-section :deep(.address) { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.validator-section dl { display: grid; gap: 0; margin: 22px 0 0; }.validator-section dl div { display: flex; justify-content: space-between; gap: 20px; padding: 12px 0; border-top: 1px solid var(--app-border); }.validator-section dt { color: var(--app-faint); }.validator-section dd { margin: 0; font-weight: 800; text-align: right; }
.empty-telemetry { min-height: 178px; display: grid; align-content: center; justify-items: center; gap: 16px; text-align: center; color: var(--app-faint); }.empty-telemetry span { width: 80%; height: 64px; border-bottom: 2px solid var(--app-border); clip-path: polygon(0 70%, 16% 55%, 29% 62%, 43% 28%, 58% 48%, 72% 20%, 86% 40%, 100% 12%, 100% 100%, 0 100%); background: color-mix(in srgb, var(--nimiq-light-blue) 10%, transparent); }.empty-telemetry p { max-width: 45ch; margin: 0; }
.activity-section a { color: var(--nimiq-light-blue); font-weight: 800; text-decoration: none; }.empty-copy { margin: 0; color: var(--app-faint); }

@media (max-width: 900px) { .metrics { grid-template-columns: 1fr 1fr; }.metrics article:nth-child(3n) { border-right: 1px solid var(--app-border); }.metrics article:nth-child(2n) { border-right: 0; }.metrics article:nth-last-child(-n+3) { border-bottom: 1px solid var(--app-border); }.metrics article:nth-last-child(-n+2) { border-bottom: 0; }.detail-grid { grid-template-columns: 1fr; }.activity-section li { grid-template-columns: 9px minmax(0, 1fr); }.activity-section .event-facts { grid-column: 2; } }
@media (max-width: 620px) { .operator-overview { gap: 16px; }.status-hero { align-items: flex-start; padding: 26px 22px; }.status-icon { display: none; }.live-control { justify-items: start; }.status-hero { flex-direction: column; }.metrics { grid-template-columns: 1fr; }.metrics article, .metrics article:nth-child(2n), .metrics article:nth-child(3n), .metrics article:nth-last-child(-n+3), .metrics article:nth-last-child(-n+2) { border-right: 0; border-bottom: 1px solid var(--app-border); }.metrics article:last-child { border-bottom: 0; }.attention-section, .detail-section, .activity-section { padding: 24px 20px; }.section-heading { align-items: flex-start; }.activity-section .section-heading { flex-direction: column; gap: 10px; } }
</style>
