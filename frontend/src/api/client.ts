const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080/api/v1'
const MEDIA_BASE_URL = API_BASE_URL.replace(/\/api\/v1\/?$/, '')

export type ApiResponse<T> = {
  data?: T
  error?: string
}

export type User = {
  id: number
  username: string
  nickname: string
  avatarUrl: string
  bio: string
}

export type AuthResult = {
  token: string
  user: User
}

export type Video = {
  id: number
  authorId: number
  title: string
  description: string
  videoUrl: string
  coverUrl: string
  publishedAt: string
  createdAt: string
  updatedAt: string
  commentCount?: number
}

export type VideoPage = {
  items: Video[]
  nextCursor: string
  hasMore: boolean
}

export type UploadResult = {
  url: string
  name: string
  size: number
}

export type UserStats = {
  userId: number
  worksCount: number
  likeCount: number
  followerCount: number
  followingCount: number
}

export type LikeStatus = {
  videoId: number
  liked: boolean
  likeCount: number
  status?: string
  operationId?: string
  opVersion?: string
}

export type FollowStatus = {
  userId: number
  following: boolean
  followerCount: number
}

export type FollowUser = {
  id: number
  username: string
  nickname: string
  avatarUrl: string
  bio: string
}

export type Comment = {
  id: string
  userId: number
  userNickname: string
  videoId: number
  content: string
  opVersion?: string
  createdAt: string
  updatedAt: string
  pending?: boolean
  pendingOperationId?: string
  pendingDelete?: boolean
}

export type PendingComment = {
  id: string
  videoId: number
  content: string
  status: string
  operationId: string
  opVersion: string
  createdAt: string
}

export type PendingCommentDelete = {
  commentId: string
  status: string
  operationId: string
  opVersion: string
}

export class ApiError extends Error {
  status: number
  code?: string

  constructor(status: number, message: string, code?: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const { headers: optionHeaders, ...rest } = options
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...rest,
    headers: {
      'Content-Type': 'application/json',
      ...(optionHeaders || {}),
    },
  })
  const body = (await response.json()) as ApiResponse<T>
  if (!response.ok) {
    const message = body.error || 'request failed'
    const code = message === 'comment_not_ready' ? 'comment_not_ready' : undefined
    throw new ApiError(response.status, message, code)
  }
  return body.data as T
}

function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

async function upload(path: string, token: string, file: File): Promise<UploadResult> {
  const formData = new FormData()
  formData.append('file', file)

  const response = await fetch(`${API_BASE_URL}${path}`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: formData,
  })
  const body = (await response.json()) as ApiResponse<UploadResult>
  if (!response.ok) {
    throw new Error(body.error || 'upload failed')
  }
  return body.data as UploadResult
}

export function resolveMediaUrl(path: string) {
  if (!path) {
    return ''
  }
  if (/^https?:\/\//.test(path)) {
    return path
  }
  return `${MEDIA_BASE_URL}${path.startsWith('/') ? path : `/${path}`}`
}

export function register(payload: { username: string; password: string; nickname?: string }) {
  return request<AuthResult>('/auth/register', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function login(payload: { username: string; password: string }) {
  return request<AuthResult>('/auth/login', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function listVideos(options: { limit?: number; cursor?: string } = {}) {
  const params = new URLSearchParams()
  if (options.limit) {
    params.set('limit', String(options.limit))
  }
  if (options.cursor) {
    params.set('cursor', options.cursor)
  }
  const query = params.toString()
  return request<VideoPage>(`/videos${query ? `?${query}` : ''}`)
}

export type HotPage = {
  items: Video[]
  snapshotId: string
  nextOffset: number
  lastVideoId: number
  hasMore: boolean
}

export function listHotVideos(options: { limit?: number; snapshotId?: string; offset?: number } = {}) {
  const params = new URLSearchParams()
  if (options.limit) {
    params.set('limit', String(options.limit))
  }
  if (options.snapshotId) {
    params.set('snapshotId', options.snapshotId)
  }
  if (options.offset !== undefined) {
    params.set('offset', String(options.offset))
  }
  const query = params.toString()
  return request<HotPage>(`/feed/hot${query ? `?${query}` : ''}`)
}

export function getVideo(id: string) {
  return request<Video>(`/videos/${id}`)
}

export function getVideoLikeStatus(id: string, token?: string) {
  return request<LikeStatus>(`/videos/${id}/like-status`, token ? {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  } : undefined)
}

export function likeVideo(token: string, id: string, idempotencyKey?: string) {
  return request<LikeStatus>(`/videos/${id}/like`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      ...(idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : {}),
    },
  })
}

export function unlikeVideo(token: string, id: string, idempotencyKey?: string) {
  return request<LikeStatus>(`/videos/${id}/like`, {
    method: 'DELETE',
    headers: {
      Authorization: `Bearer ${token}`,
      ...(idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : {}),
    },
  })
}

export { newIdempotencyKey }

export function listComments(id: string, limit = 50) {
  return request<{ items: Comment[] }>(`/videos/${id}/comments?limit=${limit}`)
}

export function createComment(token: string, id: string, content: string, idempotencyKey?: string) {
  return request<PendingComment>(`/videos/${id}/comments`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      ...(idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : {}),
    },
    body: JSON.stringify({ content }),
  })
}

export function deleteComment(token: string, videoId: string, commentId: string, idempotencyKey?: string) {
  return request<PendingCommentDelete>(`/videos/${videoId}/comments/${commentId}`, {
    method: 'DELETE',
    headers: {
      Authorization: `Bearer ${token}`,
      ...(idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : {}),
    },
  })
}

export function getUser(id: string) {
  return request<User>(`/users/${id}`)
}

export function getCurrentUser(token: string) {
  return request<User>('/users/me', {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

export function updateProfile(token: string, payload: { nickname: string; avatarUrl?: string; bio?: string }) {
  return request<User>('/users/me', {
    method: 'PATCH',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(payload),
  })
}

export function changePassword(token: string, payload: { oldPassword: string; newPassword: string }) {
  return request<{ updated: boolean }>('/users/me/password', {
    method: 'PATCH',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(payload),
  })
}

export function getUserStats(id: string) {
  return request<UserStats>(`/users/${id}/stats`)
}

export function listUserVideos(id: string, limit = 20) {
  return request<{ items: Video[] }>(`/users/${id}/videos?limit=${limit}`)
}

export function getFollowStatus(id: string, token?: string) {
  return request<FollowStatus>(`/users/${id}/follow-status`, token ? {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  } : undefined)
}

export function followUser(token: string, id: string) {
  return request<FollowStatus>(`/users/${id}/follow`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

export function unfollowUser(token: string, id: string) {
  return request<FollowStatus>(`/users/${id}/follow`, {
    method: 'DELETE',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

export function listFollowers(id: string, limit = 50) {
  return request<{ items: FollowUser[] }>(`/users/${id}/followers?limit=${limit}`)
}

export function listFollowing(id: string, limit = 50) {
  return request<{ items: FollowUser[] }>(`/users/${id}/following?limit=${limit}`)
}

export function listLikedVideos(token: string, limit = 20) {
  return request<{ items: Video[] }>(`/me/liked-videos?limit=${limit}`, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

export function uploadVideo(token: string, file: File) {
  return upload('/uploads/videos', token, file)
}

export function uploadCover(token: string, file: File) {
  return upload('/uploads/covers', token, file)
}

export function publishVideo(token: string, payload: { title: string; description?: string; videoUrl: string; coverUrl?: string }) {
  return request<Video>('/videos', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(payload),
  })
}
