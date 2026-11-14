<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import {
  ApiError,
  createComment,
  deleteComment,
  getVideo,
  getVideoLikeStatus,
  likeVideo,
  listComments,
  newIdempotencyKey,
  resolveMediaUrl,
  unlikeVideo,
  type Comment,
  type LikeStatus,
  type PendingComment,
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
const deletingCommentID = ref('')
const pendingLike = ref<{ operationId: string; opVersion: string; liked: boolean; key: string } | null>(null)
const pendingComments = ref<Map<string, PendingComment>>(new Map())
const pendingCommentKeys = ref<Map<string, string>>(new Map())

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

function parseOpVersion(value?: string): bigint {
  if (!value) return 0n
  try {
    return BigInt(value)
  } catch {
    return 0n
  }
}

function isPendingLikeCaughtUp(status: LikeStatus): boolean {
  if (!pendingLike.value) return true
  // Caught up when read version reaches/exceeds pending version and state matches target.
  return parseOpVersion(status.opVersion) >= parseOpVersion(pendingLike.value.opVersion) && status.liked === pendingLike.value.liked
}

async function pollLikeConvergence(attempts = 5, intervalMs = 2000) {
  for (let i = 0; i < attempts; i++) {
    await new Promise((resolve) => setTimeout(resolve, intervalMs))
    try {
      const status = await getVideoLikeStatus(String(video.value!.id), session.token)
      if (isPendingLikeCaughtUp(status)) {
        likeStatus.value = status
        pendingLike.value = null
        return
      }
      // Not caught up yet; update count but keep optimistic liked state
      likeStatus.value = { ...likeStatus.value!, likeCount: status.likeCount }
    } catch {
      // retry next round
    }
  }
}

async function refreshLikeStatus() {
  if (!video.value) return
  const status = await getVideoLikeStatus(String(video.value.id), session.token)
  if (isPendingLikeCaughtUp(status)) {
    likeStatus.value = status
    pendingLike.value = null
  } else {
    likeStatus.value = { ...likeStatus.value!, likeCount: status.likeCount }
  }
}

async function toggleLike() {
  if (!session.token || !video.value) {
    return
  }
  liking.value = true
  error.value = ''
  const key = newIdempotencyKey()
  const targetLiked = !likeStatus.value?.liked
  const previousStatus = likeStatus.value ? { ...likeStatus.value } : null
  // Optimistic update
  likeStatus.value = {
    ...(likeStatus.value || { videoId: video.value.id, liked: false, likeCount: 0 }),
    liked: targetLiked,
    likeCount: (likeStatus.value?.likeCount ?? 0) + (targetLiked ? 1 : -1),
    status: 'pending',
  }
  try {
    const result = targetLiked
      ? await likeVideo(session.token, String(video.value.id), key)
      : await unlikeVideo(session.token, String(video.value.id), key)
    pendingLike.value = { operationId: result.operationId || '', opVersion: result.opVersion || '', liked: targetLiked, key }
    pollLikeConvergence().catch(() => {})
  } catch (err) {
    if (err instanceof ApiError && err.status === 409) {
      pendingLike.value = null
      likeStatus.value = previousStatus
      error.value = '操作冲突，请刷新后重试'
    } else {
      // Network failure / 503 / timeout: keep optimistic state with original key for retry
      pendingLike.value = { operationId: '', opVersion: '', liked: targetLiked, key }
      error.value = '网络异常，已保留操作，可点击重试'
    }
  } finally {
    liking.value = false
  }
}

async function retryPendingLike() {
  if (!session.token || !video.value || !pendingLike.value) return
  liking.value = true
  error.value = ''
  const { key, liked } = pendingLike.value
  try {
    const result = liked
      ? await likeVideo(session.token, String(video.value.id), key)
      : await unlikeVideo(session.token, String(video.value.id), key)
    pendingLike.value = { operationId: result.operationId || '', opVersion: result.opVersion || '', liked, key }
    pollLikeConvergence().catch(() => {})
  } catch (err) {
    if (err instanceof ApiError && err.status === 409) {
      error.value = '操作冲突，请刷新后重试'
    } else {
      error.value = err instanceof Error ? err.message : '重试失败'
    }
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
  const key = newIdempotencyKey()
  const content = commentContent.value
  // Optimistically insert pending comment BEFORE request so any failure retains it for retry
  const tempId = newIdempotencyKey()
  pendingCommentKeys.value.set(tempId, key)
  comments.value.unshift({
    id: tempId,
    videoId: video.value.id,
    userId: session.user?.id ?? 0,
    userNickname: session.user?.nickname || '我',
    content,
    pending: true,
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  })
  video.value.commentCount = (video.value.commentCount ?? 0) + 1
  commentContent.value = ''
  try {
    const pending = await createComment(session.token, String(video.value.id), content, key)
    // Replace temp entry with real pending ID
    pendingCommentKeys.value.delete(tempId)
    pendingCommentKeys.value.set(pending.id, key)
    const idx = comments.value.findIndex((c) => c.id === tempId)
    if (idx >= 0) {
      comments.value[idx] = {
        id: pending.id,
        videoId: video.value.id,
        userId: session.user?.id ?? 0,
        userNickname: session.user?.nickname || '我',
        content,
        pending: true,
        pendingOperationId: pending.operationId,
        createdAt: pending.createdAt,
        updatedAt: pending.createdAt,
      }
    }
    pendingComments.value.set(pending.id, pending)
    pollCommentConvergence(pending.id).catch(() => {})
  } catch (err) {
    if (err instanceof ApiError && err.status === 409) {
      // Conflict means the key was already used; remove optimistic comment
      comments.value = comments.value.filter((c) => c.id !== tempId)
      pendingCommentKeys.value.delete(tempId)
      video.value.commentCount = Math.max(0, (video.value.commentCount ?? 1) - 1)
      error.value = '操作冲突，请刷新后重试'
    } else {
      // Network failure / 503 / timeout: keep pending comment with original key for retry
      error.value = '网络异常，已保留评论，可点击重试发送'
    }
  } finally {
    commentSubmitting.value = false
  }
}

async function retryPendingComment(tempId: string) {
  const key = pendingCommentKeys.value.get(tempId)
  const comment = comments.value.find((c) => c.id === tempId)
  if (!session.token || !video.value || !key || !comment) return
  commentSubmitting.value = true
  error.value = ''
  try {
    const pending = await createComment(session.token, String(video.value.id), comment.content, key)
    pendingCommentKeys.value.delete(tempId)
    pendingCommentKeys.value.set(pending.id, key)
    const idx = comments.value.findIndex((c) => c.id === tempId)
    if (idx >= 0) {
      comments.value[idx] = {
        ...comment,
        id: pending.id,
        pendingOperationId: pending.operationId,
      }
    }
    pendingComments.value.set(pending.id, pending)
    pollCommentConvergence(pending.id).catch(() => {})
  } catch (err) {
    if (err instanceof ApiError && err.status === 409) {
      comments.value = comments.value.filter((c) => c.id !== tempId)
      pendingCommentKeys.value.delete(tempId)
      video.value!.commentCount = Math.max(0, (video.value!.commentCount ?? 1) - 1)
      error.value = '操作冲突，请刷新后重试'
    } else {
      error.value = err instanceof Error ? err.message : '重试失败'
    }
  } finally {
    commentSubmitting.value = false
  }
}

function mergeServerItems(serverItems: Comment[]): Comment[] {
  // Preserve local pending comments and pendingDelete flags not yet reflected server-side
  const localPending = comments.value.filter((c) => c.pending && !serverItems.some((s) => s.id === c.id))
  const localDeleting = comments.value.filter((c) => c.pendingDelete && serverItems.some((s) => s.id === c.id))
  const merged = serverItems.map((s) => {
    const local = comments.value.find((c) => c.id === s.id)
    if (local?.pendingDelete) return { ...s, pendingDelete: true }
    return s
  })
  return [...localPending, ...merged]
}

async function pollCommentConvergence(commentId: string, attempts = 5, intervalMs = 2000) {
  for (let i = 0; i < attempts; i++) {
    await new Promise((resolve) => setTimeout(resolve, intervalMs))
    try {
      const result = await listComments(String(video.value!.id))
      const items = result.items || []
      const found = items.some((item) => item.id === commentId)
      if (found) {
        comments.value = mergeServerItems(items)
        pendingComments.value.delete(commentId)
        pendingCommentKeys.value.delete(commentId)
        return
      }
      comments.value = mergeServerItems(items)
    } catch {
      // retry next round
    }
  }
}

async function removeComment(comment: Comment) {
  if (!session.token || !video.value) {
    return
  }
  if (comment.pending) {
    error.value = '评论发送中，暂不能删除'
    return
  }
  deletingCommentID.value = comment.id
  error.value = ''
  // Reuse existing key if this is a retry, otherwise create new
  const existingKey = pendingCommentKeys.value.get(comment.id)
  const key = existingKey || newIdempotencyKey()
  pendingCommentKeys.value.set(comment.id, key)
  try {
    await deleteComment(session.token, String(video.value.id), comment.id, key)
    pendingCommentKeys.value.delete(comment.id)
    comment.pendingDelete = true
    pollDeleteConvergence(comment.id).catch(() => {})
  } catch (err) {
    if (err instanceof ApiError && err.code === 'comment_not_ready') {
      error.value = '评论尚未提交，请稍后再试'
    } else if (err instanceof ApiError && err.status === 409) {
      pendingCommentKeys.value.delete(comment.id)
      error.value = '操作冲突，请刷新后重试'
    } else {
      // Keep key for retry
      error.value = '网络异常，可点击重试删除'
    }
  } finally {
    deletingCommentID.value = ''
  }
}

async function pollDeleteConvergence(commentId: string, attempts = 5, intervalMs = 2000) {
  for (let i = 0; i < attempts; i++) {
    await new Promise((resolve) => setTimeout(resolve, intervalMs))
    try {
      const result = await listComments(String(video.value!.id))
      const items = result.items || []
      const stillThere = items.some((item) => item.id === commentId)
      if (!stillThere) {
        comments.value = comments.value.filter((c) => c.id !== commentId)
        video.value!.commentCount = comments.value.length
        return
      }
      // Keep pendingDelete flag via merge
      comments.value = mergeServerItems(items)
    } catch {
      // retry
    }
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
        <span>点赞数：{{ likeStatus?.likeCount ?? 0 }}{{ pendingLike ? '（同步中...）' : '' }}</span>
        <button
          v-if="pendingLike && !pendingLike.operationId"
          type="button"
          :disabled="liking || !session.token"
          @click="retryPendingLike"
        >
          {{ liking ? '重试中...' : '重试点赞' }}
        </button>
        <button v-else type="button" :disabled="liking || likeStatusLoading || !session.token" @click="toggleLike">
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
              <span>{{ comment.content }}{{ comment.pending ? '（发送中...）' : '' }}</span>
            </div>
            <button
              v-if="comment.pending && !comment.pendingOperationId && session.user?.id === comment.userId"
              type="button"
              :disabled="commentSubmitting"
              @click="retryPendingComment(comment.id)"
            >
              {{ commentSubmitting ? '重试中...' : '重试发送' }}
            </button>
            <button
              v-else-if="session.user?.id === comment.userId"
              type="button"
              :disabled="deletingCommentID === comment.id || comment.pending || comment.pendingDelete"
              @click="removeComment(comment)"
            >
              {{ comment.pendingDelete ? '删除中...' : deletingCommentID === comment.id ? '删除中...' : '删除' }}
            </button>
          </li>
        </ul>
      </section>
    </article>
  </section>
</template>
