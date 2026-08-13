import { createRouter, createWebHistory } from 'vue-router'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'pool', component: () => import('./pages/PoolOverview.vue') },
    { path: '/epochs', name: 'epochs', component: () => import('./pages/Epochs.vue') },
    { path: '/epochs/:number', name: 'epoch-detail', component: () => import('./pages/EpochDetail.vue'), props: true },
    { path: '/stakers/:address?', name: 'staker-lookup', component: () => import('./pages/StakerLookup.vue'), props: true },
    { path: '/me', name: 'my-dashboard', component: () => import('./pages/MyDashboard.vue') },
    { path: '/operator', name: 'operator', component: () => import('./pages/Operator.vue') },
  ],
})
