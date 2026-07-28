<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { listVideos, type Video } from '../api/client'

const videos = ref<Video[]>([])
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listVideos()
    videos.value = result.items || []
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="hero">
    <h1>最新视频流</h1>
    <p>按发布时间倒序展示 MVP 视频列表。</p>
  </section>

  <section class="page">
    <div class="section-title">
      <h2>视频</h2>
      <button type="button" @click="load">刷新</button>
    </div>
    <p v-if="loading">加载中...</p>
    <p v-else-if="error" class="error">{{ error }}</p>
    <p v-else-if="videos.length === 0">暂无视频。</p>
    <ul v-else class="video-list">
      <li v-for="video in videos" :key="video.id">
        <RouterLink :to="`/video/${video.id}`">{{ video.title }}</RouterLink>
        <span>
          作者：<RouterLink :to="`/users/${video.authorId}`">{{ video.authorId }}</RouterLink>
        </span>
        <span>{{ video.description || '无描述' }}</span>
      </li>
    </ul>
  </section>
</template>
