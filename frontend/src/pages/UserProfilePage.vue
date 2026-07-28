<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { getUserStats, listUserVideos, resolveMediaUrl, type UserStats, type Video } from '../api/client'

const route = useRoute()
const stats = ref<UserStats | null>(null)
const videos = ref<Video[]>([])
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const userId = String(route.params.id)
    const [statsResult, videosResult] = await Promise.all([
      getUserStats(userId),
      listUserVideos(userId),
    ])
    stats.value = statsResult
    videos.value = videosResult.items || []
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(() => route.params.id, load)
</script>

<template>
  <section class="hero">
    <h1>用户主页</h1>
    <p>查看作者统计和作品列表。</p>
  </section>

  <section class="page profile-grid">
    <article class="profile-card">
      <h2>主页统计</h2>
      <p v-if="loading">加载中...</p>
      <p v-else-if="error" class="error">{{ error }}</p>
      <dl v-else-if="stats" class="stats-list">
        <div><dt>作品数</dt><dd>{{ stats.worksCount }}</dd></div>
        <div><dt>获赞数</dt><dd>{{ stats.likeCount }}</dd></div>
        <div><dt>粉丝数</dt><dd>{{ stats.followerCount }}</dd></div>
        <div><dt>关注数</dt><dd>{{ stats.followingCount }}</dd></div>
      </dl>
    </article>

    <article class="profile-card">
      <div class="section-title">
        <h2>作者作品</h2>
        <RouterLink to="/feed">返回 Feed</RouterLink>
      </div>
      <p v-if="!loading && videos.length === 0 && !error">暂无作品。</p>
      <ul v-else class="video-list">
        <li v-for="video in videos" :key="video.id">
          <RouterLink :to="`/video/${video.id}`">{{ video.title }}</RouterLink>
          <img v-if="video.coverUrl" :src="resolveMediaUrl(video.coverUrl)" :alt="video.title" />
          <span>{{ video.description || '无描述' }}</span>
        </li>
      </ul>
    </article>
  </section>
</template>
