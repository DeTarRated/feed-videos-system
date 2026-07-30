<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { getCurrentUser, resolveMediaUrl } from './api/client'
import { useSessionStore } from './stores/session'

const session = useSessionStore()
const profileLink = computed(() => session.user ? `/users/${session.user.id}` : '/login')
const avatarLabel = computed(() => session.user?.nickname || session.user?.username || '登录')
const avatarText = computed(() => avatarLabel.value.slice(0, 1).toUpperCase())

onMounted(async () => {
  if (!session.token) {
    return
  }
  try {
    const current = await getCurrentUser(session.token)
    session.setUser(current)
  } catch {
    session.clearSession()
  }
})
</script>

<template>
  <main class="app-shell">
    <aside class="side-nav" aria-label="主导航">
      <RouterLink class="side-nav-item" to="/feed">推荐</RouterLink>
      <RouterLink class="side-nav-item" :to="profileLink">主页</RouterLink>
    </aside>

    <section class="shell">
      <nav class="topbar" aria-label="用户入口">
        <RouterLink class="topbar-avatar" :to="profileLink" :aria-label="avatarLabel">
          <img v-if="session.user?.avatarUrl" :src="resolveMediaUrl(session.user.avatarUrl)" :alt="avatarLabel" />
          <span v-else>{{ avatarText }}</span>
        </RouterLink>
      </nav>
      <router-view />
    </section>
  </main>
</template>
