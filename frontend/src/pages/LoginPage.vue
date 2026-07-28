<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { login, register } from '../api/client'
import { useSessionStore } from '../stores/session'

const router = useRouter()
const session = useSessionStore()
const mode = ref<'login' | 'register'>('login')
const username = ref('')
const password = ref('')
const nickname = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  loading.value = true
  error.value = ''
  try {
    const result = mode.value === 'login'
      ? await login({ username: username.value, password: password.value })
      : await register({ username: username.value, password: password.value, nickname: nickname.value })
    session.setSession(result.token, result.user)
    router.push('/feed')
  } catch (err) {
    error.value = err instanceof Error ? err.message : '请求失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="page narrow">
    <h1>{{ mode === 'login' ? '登录' : '注册' }}</h1>
    <form class="form" @submit.prevent="submit">
      <label>
        用户名
        <input v-model="username" autocomplete="username" required />
      </label>
      <label>
        密码
        <input v-model="password" type="password" autocomplete="current-password" required minlength="6" />
      </label>
      <label v-if="mode === 'register'">
        昵称
        <input v-model="nickname" />
      </label>
      <p v-if="error" class="error">{{ error }}</p>
      <button type="submit" :disabled="loading">{{ loading ? '提交中...' : '提交' }}</button>
    </form>
    <button class="link-button" type="button" @click="mode = mode === 'login' ? 'register' : 'login'">
      {{ mode === 'login' ? '切换到注册' : '切换到登录' }}
    </button>
  </section>
</template>
