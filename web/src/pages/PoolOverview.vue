<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { apiGet, type ApiError } from '../api'
import NimAmount from '../components/ui/NimAmount.vue'
import type { PoolStatus } from '../types/api'

const router = useRouter()
const pool = ref<PoolStatus | null>(null)
const error = ref('')
const address = ref('')

const epochLabel = computed(() => (pool.value?.epoch_status ?? '').replaceAll('_', ' '))

function findStake() {
  const value = address.value.trim()
  if (value) router.push(`/stakers/${encodeURIComponent(value)}`)
  else router.push('/stakers')
}

onMounted(async () => {
  try {
    pool.value = await apiGet<PoolStatus>('/api/pool')
  } catch (cause) {
    const err = cause as ApiError
    if (err.code === 'setup_required') {
      await router.replace('/setup')
      return
    }
    error.value = err.message || 'Pool data is temporarily unavailable.'
  }
})
</script>

<template>
  <div class="pool-home">
    <section data-section="trust" class="hero">
      <div class="hero-copy">
        <div class="live-label"><span></span> Live pool proof</div>
        <h1>Transparent Nimiq staking, built to be verified.</h1>
        <p>{{ pool?.pool_description || 'Delegate without giving up custody. Follow pool health, rewards, and every payout from one clear place.' }}</p>
        <div class="hero-actions">
          <a href="#find-stake" class="primary-action">Find my stake</a>
          <RouterLink to="/performance" class="secondary-action">View performance</RouterLink>
        </div>
        <div class="trust-points" aria-label="Pool principles">
          <span>Non-custodial</span>
          <span>Public accounting</span>
          <span>On-chain payouts</span>
        </div>
      </div>

      <div data-section="live-proof" class="proof-panel" aria-live="polite">
        <div class="proof-heading">
          <div>
            <p>Current epoch</p>
            <strong v-if="pool">{{ pool.current_epoch }}</strong>
            <span v-else class="loading-line"></span>
          </div>
          <span class="status-pill"><i></i>{{ pool ? epochLabel : 'connecting' }}</span>
        </div>
        <div class="epoch-orbit" aria-hidden="true">
          <div class="orbit-ring"><span></span></div>
          <div class="orbit-core">NIM</div>
        </div>
        <div class="proof-foot">
          <span>Live network status</span>
          <RouterLink to="/epochs">Open epoch details →</RouterLink>
        </div>
      </div>
    </section>

    <p v-if="error" class="error" role="alert">{{ error }}</p>

    <section class="proof-metrics" aria-label="Pool statistics">
      <article>
        <p>Delegated stake</p>
        <strong v-if="pool"><NimAmount :luna="pool.total_stake_luna" /></strong>
        <span v-else class="loading-line"></span>
        <small>Securing the Nimiq network</small>
      </article>
      <article>
        <p>Rewards paid</p>
        <strong v-if="pool"><NimAmount :luna="pool.total_rewards_luna" /></strong>
        <span v-else class="loading-line"></span>
        <small>Recorded across completed batches</small>
      </article>
      <article>
        <p>Stakers</p>
        <strong v-if="pool">{{ pool.num_stakers }}</strong>
        <span v-else class="loading-line"></span>
        <small>Delegating to this validator</small>
      </article>
      <article>
        <p>Pool fee</p>
        <strong v-if="pool">{{ (pool.pool_fee_percentage * 100).toFixed(2) }}%</strong>
        <span v-else class="loading-line"></span>
        <small>Deducted transparently from rewards</small>
      </article>
    </section>

    <section id="find-stake" data-section="staker-lookup" class="lookup-section">
      <div>
        <p class="section-kicker">Your position</p>
        <h2>See exactly what your stake is doing.</h2>
        <p>Enter a Nimiq address to inspect delegated stake, pool share, rewards, and payout history.</p>
      </div>
      <form class="lookup-form" @submit.prevent="findStake">
        <label for="stake-address">Nimiq address</label>
        <div class="lookup-control">
          <input id="stake-address" v-model="address" aria-label="Nimiq address" autocomplete="off" placeholder="NQ00 …" />
          <button type="submit">View position</button>
        </div>
        <small>No wallet connection required for public lookup.</small>
      </form>
    </section>

    <section class="explain-grid">
      <article data-section="reward-model" class="reward-model">
        <p class="section-kicker">Simple by design</p>
        <h2>Rewards follow your contribution.</h2>
        <p>Your share is calculated from delegated stake for each epoch. The published pool fee is deducted, and the remainder is paid back to stakers on-chain.</p>
        <ol>
          <li><span>1</span><div><strong>Delegate</strong><small>Your NIM remains under protocol custody.</small></div></li>
          <li><span>2</span><div><strong>Earn</strong><small>Validator rewards accrue by epoch and batch.</small></div></li>
          <li><span>3</span><div><strong>Verify</strong><small>Every payout remains visible in pool history.</small></div></li>
        </ol>
      </article>

      <article data-section="activity" class="activity-panel">
        <div class="activity-mark" aria-hidden="true">
          <svg viewBox="0 0 24 24"><path d="M4 17l5-5 3 3 7-8"/><path d="M14 7h5v5"/></svg>
        </div>
        <p class="section-kicker">Public operating record</p>
        <h2>Proof, not promises.</h2>
        <p>Review epoch performance, reward totals, fees, and validator history using the same data the pool operates on.</p>
        <RouterLink to="/performance">Explore verified performance <span>→</span></RouterLink>
      </article>
    </section>
  </div>
</template>

<style scoped>
.pool-home { display: grid; gap: 0; }
.hero {
  display: grid;
  grid-template-columns: minmax(0, 1.08fr) minmax(360px, .82fr);
  align-items: center;
  gap: clamp(48px, 7vw, 96px);
  min-height: 610px;
  padding: 72px 0 64px;
}
.hero-copy { max-width: 670px; }
.live-label { display: flex; align-items: center; gap: 9px; margin-bottom: 22px; color: var(--nimiq-light-blue); font-weight: 700; }
.live-label span { width: 9px; height: 9px; border-radius: 50%; background: var(--nimiq-green); box-shadow: 0 0 0 6px color-mix(in srgb, var(--nimiq-green) 16%, transparent); }
.hero h1 { max-width: 12ch; margin-bottom: 24px; font-size: clamp(3rem, 6vw, 5.4rem); line-height: .98; letter-spacing: -.04em; text-wrap: balance; }
.hero-copy > p { max-width: 60ch; margin-bottom: 30px; color: var(--app-muted); font-size: 1.14rem; line-height: 1.7; }
.hero-actions { display: flex; gap: 12px; flex-wrap: wrap; margin-bottom: 32px; }
.primary-action, .secondary-action { min-height: 50px; display: inline-flex; align-items: center; justify-content: center; padding: 0 24px; border-radius: 999px; font-weight: 700; text-decoration: none; }
.primary-action { color: white; background: var(--nimiq-light-blue-bg); box-shadow: 0 12px 28px rgba(5,130,202,.24); }
.secondary-action { color: var(--app-text); border: 1px solid var(--app-border); background: var(--surface-1); }
.trust-points { display: flex; gap: 22px; flex-wrap: wrap; color: var(--app-faint); font-size: .82rem; font-weight: 700; }
.trust-points span::before { content: '✓'; margin-right: 7px; color: var(--nimiq-green); }
.proof-panel {
  min-height: 460px;
  display: flex;
  flex-direction: column;
  padding: 32px;
  overflow: hidden;
  color: white;
  border-radius: 20px;
  background: var(--nimiq-blue-bg);
  box-shadow: 0 30px 70px rgba(31,35,72,.26);
}
.proof-heading, .proof-foot { display: flex; justify-content: space-between; align-items: center; gap: 16px; }
.proof-heading p { margin: 0 0 6px; color: rgba(255,255,255,.58); font-size: .82rem; font-weight: 700; }
.proof-heading strong { font-size: 2.5rem; letter-spacing: -.04em; }
.status-pill { display: inline-flex; align-items: center; gap: 8px; padding: 8px 12px; border-radius: 999px; color: rgba(255,255,255,.84); background: rgba(255,255,255,.09); font-size: .75rem; font-weight: 700; text-transform: capitalize; }
.status-pill i { width: 7px; height: 7px; border-radius: 50%; background: var(--nimiq-green); }
.epoch-orbit { position: relative; flex: 1; display: grid; place-items: center; }
.orbit-ring { width: 210px; height: 210px; position: relative; display: grid; place-items: center; border: 1px solid rgba(255,255,255,.12); border-radius: 50%; box-shadow: inset 0 0 0 38px rgba(255,255,255,.025); }
.orbit-ring::before, .orbit-ring::after { content: ''; position: absolute; border: 1px solid rgba(255,255,255,.08); border-radius: 50%; }
.orbit-ring::before { inset: 24px; }.orbit-ring::after { inset: 54px; }
.orbit-ring span { position: absolute; top: 17px; right: 31px; width: 15px; height: 15px; border: 3px solid var(--nimiq-gold); border-radius: 50%; box-shadow: 0 0 24px var(--nimiq-gold); }
.orbit-core { position: absolute; font-weight: 800; letter-spacing: .08em; }
.proof-foot { color: rgba(255,255,255,.5); font-size: .78rem; }
.proof-foot a { color: white; font-weight: 700; text-decoration: none; }
.proof-metrics { display: grid; grid-template-columns: repeat(4, 1fr); border-top: 1px solid var(--app-border); border-bottom: 1px solid var(--app-border); }
.proof-metrics article { min-width: 0; padding: 32px 28px; border-right: 1px solid var(--app-border); }
.proof-metrics article:first-child { padding-left: 0; }.proof-metrics article:last-child { padding-right: 0; border-right: 0; }
.proof-metrics p { margin-bottom: 9px; color: var(--app-faint); font-size: .78rem; font-weight: 700; }
.proof-metrics strong { display: block; overflow: hidden; font-size: clamp(1.25rem, 2vw, 1.75rem); letter-spacing: -.025em; text-overflow: ellipsis; }
.proof-metrics small { display: block; margin-top: 8px; color: var(--app-faint); }
.lookup-section { display: grid; grid-template-columns: .9fr 1.1fr; align-items: center; gap: 64px; margin: 96px 0; padding: 56px; border-radius: 20px; color: white; background: var(--nimiq-light-blue-bg); box-shadow: 0 24px 56px rgba(5,130,202,.18); }
.section-kicker { margin-bottom: 12px; color: var(--nimiq-light-blue); font-weight: 700; }
.lookup-section .section-kicker { color: rgba(255,255,255,.62); }
.lookup-section h2, .explain-grid h2 { margin-bottom: 16px; font-size: clamp(1.8rem, 3.5vw, 2.8rem); letter-spacing: -.035em; }
.lookup-section p:not(.section-kicker) { max-width: 48ch; color: rgba(255,255,255,.72); }
.lookup-form { padding: 24px; border-radius: 14px; background: white; color: var(--nimiq-blue); box-shadow: 0 14px 40px rgba(0,0,0,.12); }
.lookup-form label { display: block; margin-bottom: 9px; font-size: .82rem; font-weight: 700; }
.lookup-control { display: flex; gap: 8px; }
.lookup-control input { min-width: 0; flex: 1; padding: 14px 16px; border: 1px solid rgba(31,35,72,.14); border-radius: 10px; color: var(--nimiq-blue); background: #fff; font: .9rem var(--font-mono); }
.lookup-control button { padding: 0 18px; border: 0; border-radius: 9px; color: white; background: var(--nimiq-blue); font: inherit; font-weight: 700; cursor: pointer; }
.lookup-form small { display: block; margin-top: 10px; color: rgba(31,35,72,.52); }
.explain-grid { display: grid; grid-template-columns: 1.15fr .85fr; gap: 32px; }
.reward-model, .activity-panel { padding: 48px; border-radius: 18px; background: var(--surface-1); box-shadow: var(--shadow-elevated); }
.reward-model > p:not(.section-kicker), .activity-panel > p:not(.section-kicker) { color: var(--app-muted); line-height: 1.7; }
.reward-model ol { display: grid; gap: 18px; margin: 28px 0 0; padding: 0; list-style: none; }
.reward-model li { display: flex; align-items: center; gap: 16px; }
.reward-model li > span { width: 34px; height: 34px; display: grid; place-items: center; flex: 0 0 auto; border-radius: 50%; color: white; background: var(--nimiq-blue); font-weight: 800; }
.reward-model li div { display: grid; }.reward-model li small { margin-top: 3px; color: var(--app-faint); }
.activity-panel { display: flex; flex-direction: column; align-items: flex-start; background: var(--surface-2); }
.activity-mark { width: 62px; height: 62px; display: grid; place-items: center; margin-bottom: 38px; border-radius: 16px; color: white; background: var(--nimiq-green-bg); }
.activity-mark svg { width: 29px; fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; }
.activity-panel a { margin-top: auto; padding-top: 32px; color: var(--nimiq-light-blue); font-weight: 800; text-decoration: none; }.activity-panel a span { margin-left: 8px; }
.loading-line { display: block; width: 110px; height: 28px; border-radius: 7px; background: var(--surface-2); animation: pulse 1.3s ease-in-out infinite alternate; }
.proof-panel .loading-line { background: rgba(255,255,255,.12); }
@keyframes pulse { to { opacity: .45; } }

@media (max-width: 900px) {
  .hero { grid-template-columns: 1fr; min-height: auto; padding-top: 56px; }
  .hero-copy { max-width: 760px; }.hero h1 { max-width: 13ch; }
  .proof-panel { min-height: 400px; }
  .proof-metrics { grid-template-columns: 1fr 1fr; }.proof-metrics article { border-bottom: 1px solid var(--app-border); }.proof-metrics article:nth-child(2) { border-right: 0; }.proof-metrics article:nth-last-child(-n+2) { border-bottom: 0; }.proof-metrics article:nth-child(3) { padding-left: 0; }
  .lookup-section, .explain-grid { grid-template-columns: 1fr; }
}
@media (max-width: 620px) {
  .hero { padding: 44px 0; gap: 36px; }.hero h1 { font-size: 3rem; }
  .proof-panel { min-height: 380px; padding: 24px; }.orbit-ring { width: 180px; height: 180px; }
  .proof-metrics { grid-template-columns: 1fr; }.proof-metrics article, .proof-metrics article:first-child, .proof-metrics article:nth-child(3), .proof-metrics article:last-child { padding: 22px 0; border-right: 0; border-bottom: 1px solid var(--app-border); }.proof-metrics article:last-child { border-bottom: 0; }
  .lookup-section { gap: 28px; margin: 64px -8px; padding: 32px 24px; border-radius: 16px; }
  .lookup-control { flex-direction: column; }.lookup-control button { min-height: 46px; }
  .reward-model, .activity-panel { padding: 32px 24px; }
}
</style>
