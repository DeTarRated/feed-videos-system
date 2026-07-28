import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/', redirect: '/feed' },
  { path: '/login', component: () => import('../pages/LoginPage.vue') },
  { path: '/feed', component: () => import('../pages/FeedPage.vue') },
  { path: '/publish', component: () => import('../pages/PublishPage.vue') },
  { path: '/video/:id', component: () => import('../pages/VideoDetailPage.vue') },
  { path: '/users/:id', component: () => import('../pages/UserProfilePage.vue') },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
