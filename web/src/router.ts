import { createRouter, createWebHistory } from 'vue-router'
import PublicLayout from './layouts/PublicLayout.vue'
import OperatorLayout from './layouts/OperatorLayout.vue'
import SetupLayout from './layouts/SetupLayout.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      component: PublicLayout,
      children: [
        { path: '', name: 'pool', component: () => import('./pages/PoolOverview.vue') },
        { path: 'epochs', name: 'epochs', component: () => import('./pages/Epochs.vue') },
        { path: 'epochs/:number', name: 'epoch-detail', component: () => import('./pages/EpochDetail.vue'), props: true },
        { path: 'stakers/:address?', name: 'staker-lookup', component: () => import('./pages/StakerLookup.vue'), props: true },
        { path: 'onboard', name: 'onboard', component: () => import('./pages/Onboard.vue') },
        { path: 'me', name: 'my-dashboard', component: () => import('./pages/MyDashboard.vue') },
        { path: 'rewards', name: 'rewards', component: () => import('./pages/RewardsChart.vue') },
      ]
    },
    {
      path: '/operator',
      component: OperatorLayout,
      children: [
        { path: '', name: 'operator', component: () => import('./pages/Operator.vue') },
      ]
    },
    {
      path: '/setup',
      component: SetupLayout,
      children: []
    }
  ],
})
