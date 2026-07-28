<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { getVideo, resolveMediaUrl, type Video } from '../api/client'

const route = useRoute()
const video = ref<Video | null>(null)
const error = ref('')
const loading = ref(false)

onMounted(async () => {
  loading.value = true
  try {
    video.value = await getVideo(String(route.params.id))
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <section class="page">
    <p v-if="loading">加载中...</p>
    <p v-else-if="error" class="error">{{ error }}</p>
    <article v-else-if="video" class="detail">
      <h1>{{ video.title }}</h1>
      <p>{{ video.description || '无描述' }}</p>
      <video :src="resolveMediaUrl(video.videoUrl)" :poster="resolveMediaUrl(video.coverUrl)" controls playsinline />
    </article>
  </section>
</template>
