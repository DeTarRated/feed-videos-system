<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { listHotVideos, resolveMediaUrl, type Video } from '../api/client'

const items = ref<Video[]>([])
const snapshotId = ref('')
const nextOffset = ref(0)
const hasMore = ref(false)
const loading = ref(false)
const loadingMore = ref(false)
const error = ref('')
const activeIndex = ref(0)

const activeVideo = computed(() => items.value[activeIndex.value] || null)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listHotVideos({ limit: 10 })
    items.value = result.items || []
    snapshotId.value = result.snapshotId || ''
    nextOffset.value = result.nextOffset || 0
    hasMore.value = result.hasMore
    activeIndex.value = 0
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function loadMore() {
  if (!hasMore.value || !snapshotId.value || loadingMore.value) {
    return
  }
  loadingMore.value = true
  error.value = ''
  try {
    const result = await listHotVideos({ limit: 10, snapshotId: snapshotId.value, offset: nextOffset.value })
    if (result.snapshotId !== snapshotId.value) {
      // Snapshot changed (expired); replace list with new snapshot's first page.
      items.value = result.items || []
      snapshotId.value = result.snapshotId || ''
      activeIndex.value = 0
    } else {
      items.value = [...items.value, ...(result.items || [])]
    }
    nextOffset.value = result.nextOffset || nextOffset.value
    hasMore.value = result.hasMore
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loadingMore.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="tiktok-home">
    <main class="feed-stage">
      <div class="feed-toolbar">
        <div>
          <h1>热点</h1>
          <p>5 分钟快照内稳定浏览近期热门视频</p>
        </div>
        <button type="button" @click="load">刷新</button>
      </div>

      <p v-if="loading" class="feed-state">加载中...</p>
      <p v-else-if="error" class="error">{{ error }}</p>
      <p v-else-if="items.length === 0" class="feed-state">暂无热点视频。</p>

      <article v-else-if="activeVideo" class="spotlight-card">
        <div class="phone-video">
          <video
            :src="resolveMediaUrl(activeVideo.videoUrl)"
            :poster="resolveMediaUrl(activeVideo.coverUrl)"
            controls
            playsinline
          />
          <div class="video-gradient">
            <RouterLink :to="`/users/${activeVideo.authorId}`">@user{{ activeVideo.authorId }}</RouterLink>
            <h2>{{ activeVideo.title }}</h2>
            <p>{{ activeVideo.description || '无描述' }}</p>
          </div>
        </div>

        <div class="action-rail" aria-label="视频操作">
          <RouterLink :to="`/video/${activeVideo.id}`">详情</RouterLink>
          <RouterLink :to="`/users/${activeVideo.authorId}`">作者</RouterLink>
        </div>
      </article>

      <section v-if="items.length > 1" class="video-strip" aria-label="视频列表">
        <button
          v-for="(video, index) in items"
          :key="video.id"
          type="button"
          :class="{ active: index === activeIndex }"
          @click="activeIndex = index"
        >
          <span>{{ video.title }}</span>
          <small>@user{{ video.authorId }}</small>
        </button>
      </section>

      <button v-if="hasMore" class="load-more-button" type="button" :disabled="loadingMore" @click="loadMore">
        {{ loadingMore ? '加载中...' : '加载更多' }}
      </button>
    </main>
  </section>
</template>
