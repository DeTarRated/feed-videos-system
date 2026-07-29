<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import {
  followUser,
  getFollowStatus,
  getUserStats,
  listFollowers,
  listFollowing,
  listUserVideos,
  resolveMediaUrl,
  unfollowUser,
  type FollowStatus,
  type FollowUser,
  type UserStats,
  type Video,
} from '../api/client'
import { useSessionStore } from '../stores/session'

const route = useRoute()
const session = useSessionStore()
const stats = ref<UserStats | null>(null)
const followStatus = ref<FollowStatus | null>(null)
const videos = ref<Video[]>([])
const followers = ref<FollowUser[]>([])
const following = ref<FollowUser[]>([])
const loading = ref(false)
const followingLoading = ref(false)
const error = ref('')

const profileUserId = computed(() => Number(route.params.id))
const isSelfProfile = computed(() => session.user?.id === profileUserId.value)
const followButtonLabel = computed(() => followStatus.value?.following ? '取消关注' : '关注')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const userId = String(route.params.id)
    const [statsResult, videosResult, statusResult, followersResult, followingResult] = await Promise.all([
      getUserStats(userId),
      listUserVideos(userId),
      getFollowStatus(userId, session.token),
      listFollowers(userId),
      listFollowing(userId),
    ])
    stats.value = statsResult
    followStatus.value = statusResult
    videos.value = videosResult.items || []
    followers.value = followersResult.items || []
    following.value = followingResult.items || []
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function toggleFollow() {
  if (!session.token || isSelfProfile.value) {
    return
  }
  followingLoading.value = true
  error.value = ''
  try {
    const userId = String(route.params.id)
    const next = followStatus.value?.following
      ? await unfollowUser(session.token, userId)
      : await followUser(session.token, userId)
    followStatus.value = next
    if (stats.value) {
      stats.value.followerCount = next.followerCount
    }
    const followersResult = await listFollowers(userId)
    followers.value = followersResult.items || []
  } catch (err) {
    error.value = err instanceof Error ? err.message : '操作失败'
  } finally {
    followingLoading.value = false
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
      <div class="section-title">
        <h2>主页统计</h2>
        <button
          v-if="session.token && !isSelfProfile"
          type="button"
          :disabled="followingLoading"
          @click="toggleFollow"
        >
          {{ followingLoading ? '处理中...' : followButtonLabel }}
        </button>
      </div>
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

    <article class="profile-card">
      <h2>粉丝</h2>
      <p v-if="!loading && followers.length === 0 && !error">暂无粉丝。</p>
      <ul v-else class="user-list">
        <li v-for="user in followers" :key="user.id">
          <RouterLink :to="`/users/${user.id}`">{{ user.nickname || user.username }}</RouterLink>
          <span>{{ user.bio || user.username }}</span>
        </li>
      </ul>
    </article>

    <article class="profile-card">
      <h2>关注</h2>
      <p v-if="!loading && following.length === 0 && !error">暂无关注。</p>
      <ul v-else class="user-list">
        <li v-for="user in following" :key="user.id">
          <RouterLink :to="`/users/${user.id}`">{{ user.nickname || user.username }}</RouterLink>
          <span>{{ user.bio || user.username }}</span>
        </li>
      </ul>
    </article>
  </section>
</template>
