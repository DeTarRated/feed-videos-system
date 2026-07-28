<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { getVideo, getVideoLikeStatus, likeVideo, resolveMediaUrl, unlikeVideo, type LikeStatus, type Video } from '../api/client'
import { useSessionStore } from '../stores/session'

const route = useRoute()
const session = useSessionStore()
const video = ref<Video | null>(null)
const likeStatus = ref<LikeStatus | null>(null)
const error = ref('')
const loading = ref(false)
const liking = ref(false)
const likeStatusLoading = ref(true)

const likeButtonLabel = computed(() => {
  if (likeStatusLoading.value) {
    return '读取状态...'
  }
  return likeStatus.value?.liked ? '取消点赞' : '点赞'
})

async function load() {
  loading.value = true
  likeStatusLoading.value = true
  error.value = ''
  try {
    const id = String(route.params.id)
    const [videoResult, statusResult] = await Promise.all([
      getVideo(id),
      getVideoLikeStatus(id, session.token),
    ])
    video.value = videoResult
    likeStatus.value = statusResult
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    likeStatusLoading.value = false
    loading.value = false
  }
}

async function toggleLike() {
  if (!session.token || !video.value) {
    return
  }
  liking.value = true
  try {
    const next = likeStatus.value?.liked
      ? await unlikeVideo(session.token, String(video.value.id))
      : await likeVideo(session.token, String(video.value.id))
    likeStatus.value = next
  } catch (err) {
    error.value = err instanceof Error ? err.message : '操作失败'
  } finally {
    liking.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="page">
    <p v-if="loading">加载中...</p>
    <p v-else-if="error" class="error">{{ error }}</p>
    <article v-else-if="video" class="detail">
      <h1>{{ video.title }}</h1>
      <p>{{ video.description || '无描述' }}</p>
      <video :src="resolveMediaUrl(video.videoUrl)" :poster="resolveMediaUrl(video.coverUrl)" controls playsinline />
      <div class="section-title">
        <span>点赞数：{{ likeStatus?.likeCount ?? 0 }}</span>
        <button type="button" :disabled="liking || likeStatusLoading || !session.token" @click="toggleLike">
          {{ likeButtonLabel }}
        </button>
      </div>
    </article>
  </section>
</template>
