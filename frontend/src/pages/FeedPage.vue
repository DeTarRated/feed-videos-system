<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { listHotVideos, listVideos, resolveMediaUrl, type Video } from '../api/client'

const videos = ref<Video[]>([])
const hotVideos = ref<Video[]>([])
const activeIndex = ref(0)
const nextCursor = ref('')
const hasMore = ref(false)
const loading = ref(false)
const loadingMore = ref(false)
const error = ref('')

const activeVideo = computed(() => videos.value[activeIndex.value] || null)
const sidebarVideos = computed(() => hotVideos.value.slice(0, 5))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [latestResult, hotResult] = await Promise.all([
      listVideos({ limit: 10 }),
      listHotVideos(5),
    ])
    videos.value = latestResult.items || []
    nextCursor.value = latestResult.nextCursor || ''
    hasMore.value = latestResult.hasMore
    hotVideos.value = hotResult.items || []
    activeIndex.value = 0
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function loadMore() {
  if (!hasMore.value || !nextCursor.value || loadingMore.value) {
    return
  }
  loadingMore.value = true
  error.value = ''
  try {
    const result = await listVideos({ limit: 10, cursor: nextCursor.value })
    videos.value = [...videos.value, ...(result.items || [])]
    nextCursor.value = result.nextCursor || ''
    hasMore.value = result.hasMore
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loadingMore.value = false
  }
}

function selectVideo(index: number) {
  activeIndex.value = index
}

onMounted(load)
</script>

<template>
  <section class="tiktok-home">
    <main class="feed-stage">
      <div class="feed-toolbar">
        <div>
          <h1>推荐</h1>
          <p>发现最新发布的视频</p>
        </div>
        <button type="button" @click="load">刷新</button>
      </div>

      <p v-if="loading" class="feed-state">加载中...</p>
      <p v-else-if="error" class="error">{{ error }}</p>
      <p v-else-if="videos.length === 0" class="feed-state">暂无视频。</p>

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
          <button type="button" @click="selectVideo((activeIndex + 1) % videos.length)">下一个</button>
        </div>
      </article>

      <section v-if="videos.length > 1" class="video-strip" aria-label="视频列表">
        <button
          v-for="(video, index) in videos"
          :key="video.id"
          type="button"
          :class="{ active: index === activeIndex }"
          @click="selectVideo(index)"
        >
          <span>{{ video.title }}</span>
          <small>@user{{ video.authorId }}</small>
        </button>
      </section>

      <button v-if="hasMore" class="load-more-button" type="button" :disabled="loadingMore" @click="loadMore">
        {{ loadingMore ? '加载中...' : '加载更多' }}
      </button>
    </main>

    <aside class="feed-discovery">
      <section>
        <h2>创作入口</h2>
        <p>上传视频、填写标题和封面，发布到推荐流。</p>
        <RouterLink to="/publish">发布视频</RouterLink>
      </section>

      <section>
        <h2>热榜视频</h2>
        <p v-if="sidebarVideos.length === 0">互动后会生成热榜排序。</p>
        <ul v-else>
          <li v-for="video in sidebarVideos" :key="video.id">
            <RouterLink :to="`/video/${video.id}`">{{ video.title }}</RouterLink>
            <span>@user{{ video.authorId }}</span>
          </li>
        </ul>
      </section>
    </aside>
  </section>
</template>
