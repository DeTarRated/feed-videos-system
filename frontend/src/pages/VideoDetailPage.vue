<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import {
  createComment,
  deleteComment,
  getVideo,
  getVideoLikeStatus,
  likeVideo,
  listComments,
  resolveMediaUrl,
  unlikeVideo,
  type Comment,
  type LikeStatus,
  type Video,
} from '../api/client'
import { useSessionStore } from '../stores/session'

const route = useRoute()
const session = useSessionStore()
const video = ref<Video | null>(null)
const likeStatus = ref<LikeStatus | null>(null)
const comments = ref<Comment[]>([])
const commentContent = ref('')
const error = ref('')
const loading = ref(false)
const liking = ref(false)
const likeStatusLoading = ref(true)
const commentsLoading = ref(true)
const commentSubmitting = ref(false)
const deletingCommentID = ref(0)

const likeButtonLabel = computed(() => {
  if (likeStatusLoading.value) {
    return '读取状态...'
  }
  return likeStatus.value?.liked ? '取消点赞' : '点赞'
})

async function load() {
  loading.value = true
  likeStatusLoading.value = true
  commentsLoading.value = true
  error.value = ''
  try {
    const id = String(route.params.id)
    const [videoResult, statusResult, commentsResult] = await Promise.all([
      getVideo(id),
      getVideoLikeStatus(id, session.token),
      listComments(id),
    ])
    video.value = videoResult
    likeStatus.value = statusResult
    comments.value = commentsResult.items || []
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    likeStatusLoading.value = false
    commentsLoading.value = false
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

async function submitComment() {
  if (!session.token || !video.value || !commentContent.value.trim()) {
    return
  }
  commentSubmitting.value = true
  error.value = ''
  try {
    const comment = await createComment(session.token, String(video.value.id), commentContent.value)
    comments.value.unshift(comment)
    video.value.commentCount = (video.value.commentCount ?? comments.value.length - 1) + 1
    commentContent.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : '评论失败'
  } finally {
    commentSubmitting.value = false
  }
}

async function removeComment(comment: Comment) {
  if (!session.token || !video.value) {
    return
  }
  deletingCommentID.value = comment.id
  error.value = ''
  try {
    await deleteComment(session.token, String(video.value.id), comment.id)
    comments.value = comments.value.filter((item) => item.id !== comment.id)
    video.value.commentCount = Math.max(0, (video.value.commentCount ?? comments.value.length + 1) - 1)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '删除失败'
  } finally {
    deletingCommentID.value = 0
  }
}

onMounted(load)
</script>

<template>
  <section class="page">
    <p v-if="loading">加载中...</p>
    <p v-else-if="error && !video" class="error">{{ error }}</p>
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

      <section class="comments-section">
        <div class="section-title">
          <h2>评论</h2>
          <span>{{ video.commentCount ?? comments.length }} 条</span>
        </div>
        <form v-if="session.token" class="comment-form" @submit.prevent="submitComment">
          <textarea v-model="commentContent" rows="3" maxlength="500" placeholder="写下你的评论" />
          <button type="submit" :disabled="commentSubmitting || !commentContent.trim()">
            {{ commentSubmitting ? '发送中...' : '发表评论' }}
          </button>
        </form>
        <p v-else>登录后可以发表评论。</p>
        <p v-if="error" class="error">{{ error }}</p>
        <p v-if="commentsLoading">评论加载中...</p>
        <p v-else-if="comments.length === 0">暂无评论。</p>
        <ul v-else class="comment-list">
          <li v-for="comment in comments" :key="comment.id">
            <div>
              <strong>{{ comment.userNickname || `用户 ${comment.userId}` }}</strong>
              <span>{{ comment.content }}</span>
            </div>
            <button
              v-if="session.user?.id === comment.userId"
              type="button"
              :disabled="deletingCommentID === comment.id"
              @click="removeComment(comment)"
            >
              {{ deletingCommentID === comment.id ? '删除中...' : '删除' }}
            </button>
          </li>
        </ul>
      </section>
    </article>
  </section>
</template>
